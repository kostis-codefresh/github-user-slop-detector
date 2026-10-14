// contributor-report-web is the web front end for the contributor report. It
// runs the same fetch, metrics and renderer as the CLI, with the GitHub token
// taken from the server's GH_TOKEN, and keeps the latest reports in memory.
package main

import (
	"bytes"
	"context"
	"errors"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"time"

	"github.com/kostis-codefresh/github-user-slop-detector/pkg/analysis"
	"github.com/kostis-codefresh/github-user-slop-detector/pkg/metrics"
	"github.com/kostis-codefresh/github-user-slop-detector/pkg/report"
	"github.com/kostis-codefresh/github-user-slop-detector/ui"
)

// analyzeFunc produces a fresh report for a login. It is a parameter of
// newHandler so the tests can count calls without reaching GitHub.
type analyzeFunc func(ctx context.Context, username string) (metrics.Report, error)

// loginPattern is the shape of a GitHub login. Anything else cannot be a user,
// so it is rejected before it costs an API call.
var loginPattern = regexp.MustCompile(`^[A-Za-z0-9-]{1,39}$`)

var errInvalidLogin = errors.New("not a valid GitHub username")

// errReadOnly is a cache miss with no GitHub token to analyze it with.
var errReadOnly = errors.New("read only mode")

// loggedInUser is who the banner says is logged in. A placeholder until the
// GitHub OAuth flow supplies the real login along with the token.
var loggedInUser = "demo-user"

// version is overridden at build time with -ldflags "-X main.version=...".
var version = "dev"

type pageData struct {
	Reports  []metrics.Report
	Error    string
	Version  string
	ReadOnly bool
	User     string
}

// newHandler serves the app. A nil analyze is read-only mode: cached reports
// are still served, but nothing new can be analyzed.
func newHandler(analyze analyzeFunc) http.Handler {
	readOnly := analyze == nil
	tmpl := template.Must(template.ParseFS(ui.Embedded, "index.html"))
	cache := newReportCache()

	// lookup serves a cached report, or analyzes and caches a new one. Failures
	// are not cached, so a retry after a transient error fetches again.
	lookup := func(ctx context.Context, username string) (metrics.Report, error) {
		if !loginPattern.MatchString(username) {
			return metrics.Report{}, errInvalidLogin
		}
		if r, ok := cache.get(username); ok {
			return r, nil
		}
		if readOnly {
			return metrics.Report{}, errReadOnly
		}
		r, err := analyze(ctx, username)
		if err != nil {
			return metrics.Report{}, err
		}
		cache.add(r)
		return r, nil
	}

	execute := func(w http.ResponseWriter, name string, data pageData) {
		if err := tmpl.ExecuteTemplate(w, name, data); err != nil {
			log.Printf("rendering %s: %v", name, err)
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		username := r.FormValue("username")
		if username == "" {
			data := pageData{Reports: cache.list(), Version: version, ReadOnly: readOnly}
			if !readOnly {
				data.User = loggedInUser
			}
			execute(w, "index.html", data)
			return
		}
		target := "/report/" + url.PathEscape(username)
		// Without htmx the form is a plain GET, and /report/<username> does the work.
		if r.Header.Get("HX-Request") != "true" {
			http.Redirect(w, r, target, http.StatusSeeOther)
			return
		}
		// With htmx the analysis runs here, while the form shows its indicator,
		// and the redirect then lands on a cache hit. Errors use 200 because
		// htmx does not swap 4xx and 5xx responses by default.
		if _, err := lookup(r.Context(), username); err != nil {
			log.Printf("analyzing %s: %v", username, err)
			execute(w, "analyze-error", pageData{Error: errorMessage(username, err)})
			return
		}
		w.Header().Set("HX-Redirect", target)
	})
	// Liveness only: it makes no GitHub call, so a GitHub outage does not get
	// the app restarted by whatever is probing it.
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}` + "\n"))
	})
	mux.HandleFunc("GET /img/logo.svg", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/svg+xml")
		_, _ = w.Write(report.Logo)
	})
	mux.HandleFunc("GET /report/{username}", func(w http.ResponseWriter, r *http.Request) {
		username := r.PathValue("username")
		rep, err := lookup(r.Context(), username)
		switch {
		case errors.Is(err, errInvalidLogin), errors.Is(err, analysis.ErrUserNotFound):
			http.Error(w, errorMessage(username, err), http.StatusNotFound)
			return
		case errors.Is(err, errReadOnly):
			http.Error(w, errorMessage(username, err), http.StatusForbidden)
			return
		case err != nil:
			log.Printf("analyzing %s: %v", username, err)
			http.Error(w, errorMessage(username, err), http.StatusBadGateway)
			return
		}
		// Render into a buffer so a template failure is a clean 500, not half a page.
		var buf bytes.Buffer
		if err := report.Write(&buf, rep, report.DetailsAll, "/"); err != nil {
			log.Printf("rendering report for %s: %v", username, err)
			http.Error(w, "could not render the report", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = buf.WriteTo(w)
	})
	return mux
}

// errorMessage is what a visitor sees when a lookup fails.
func errorMessage(username string, err error) string {
	switch {
	case errors.Is(err, errInvalidLogin):
		return username + " is not a valid GitHub username."
	case errors.Is(err, analysis.ErrUserNotFound):
		return "No GitHub user named " + username + "."
	case errors.Is(err, errReadOnly):
		return "Read only mode. Login with GitHub to start analyzing usernames."
	default:
		return "Could not analyze " + username + ": " + err.Error()
	}
}

// githubAnalyzer runs the same pipeline as the CLI with its default flags: the
// fetch with window fallback, then every metric.
func githubAnalyzer(token string) analyzeFunc {
	client := analysis.NewClient(token)
	return func(ctx context.Context, username string) (metrics.Report, error) {
		cfg := metrics.DefaultConfig()
		contributor, months, err := client.FetchWithFallback(ctx, username, cfg.AnalysisWindow)
		if err != nil {
			return metrics.Report{}, err
		}
		cfg.AnalysisWindow = months
		rl := contributor.RateLimit
		log.Printf("analyzed %s over %d months; GitHub API rate limit: %d points remaining (%d used), resets at %s",
			contributor.Login, months, rl.Remaining, rl.Used, rl.ResetAt.Format(time.RFC3339))
		return metrics.Evaluate(contributor, cfg, nil), nil
	}
}

func main() {
	// Without a token the GraphQL API rejects every request, so the app starts
	// read-only rather than offering an Analyze button that can only fail.
	var analyze analyzeFunc
	if token := os.Getenv("GH_TOKEN"); token != "" {
		analyze = githubAnalyzer(token)
	} else {
		log.Print("GH_TOKEN is not set: starting in read only mode. " +
			"Try: GH_TOKEN=$(gh auth token) contributor-report-web")
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           newHandler(analyze),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("listening on %s", srv.Addr)
	log.Fatal(srv.ListenAndServe())
}
