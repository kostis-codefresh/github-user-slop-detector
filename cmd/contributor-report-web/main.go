// contributor-report-web is the web front end for the contributor report. It
// runs the same fetch, metrics and renderer as the CLI, with the GitHub token
// of the user who logged in through GitHub OAuth, and keeps the latest reports
// in memory.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/kostis-codefresh/github-user-slop-detector/pkg/analysis"
	"github.com/kostis-codefresh/github-user-slop-detector/pkg/metrics"
	"github.com/kostis-codefresh/github-user-slop-detector/pkg/report"
	"github.com/kostis-codefresh/github-user-slop-detector/ui"
)

// analyzeFunc produces a fresh report for a login, using the GitHub token of
// whoever asked for it. It is a parameter of newHandler so the tests can count
// calls without reaching GitHub.
type analyzeFunc func(ctx context.Context, token, username string) (metrics.Report, error)

// loginPattern is the shape of a GitHub login. Anything else cannot be a user,
// so it is rejected before it costs an API call.
var loginPattern = regexp.MustCompile(`^[A-Za-z0-9-]{1,39}$`)

var errInvalidLogin = errors.New("not a valid GitHub username")

// errReadOnly is a cache miss with no GitHub token to analyze it with: the
// visitor is not logged in.
var errReadOnly = errors.New("read only mode")

// version is overridden at build time with -ldflags "-X main.version=...".
var version = "dev"

type pageData struct {
	Reports      []metrics.Report
	Error        string
	Version      string
	ReadOnly     bool
	LoginEnabled bool
	User         string
}

// healthStatus is the /health body. Besides liveness it says which OAuth
// settings are missing and which base URL GitHub redirects back to, the two
// things a login that does not work usually comes down to.
type healthStatus struct {
	Status                string   `json:"status"`
	GitHubClientIDSet     bool     `json:"github_client_id_set"`
	GitHubClientSecretSet bool     `json:"github_client_secret_set"`
	Missing               []string `json:"missing"`
	AppBaseURL            string   `json:"app_base_url"`
}

// newHandler serves the app. Visitors who are not logged in are read-only:
// cached reports are still served, but nothing new can be analyzed. Without
// the OAuth settings in cfg nobody can log in, so the whole app is read-only.
func newHandler(analyze analyzeFunc, cfg authConfig) http.Handler {
	tmpl := template.Must(template.ParseFS(ui.Embedded, "*.html"))
	cache := newReportCache()

	execute := func(w http.ResponseWriter, name string, data pageData) {
		if err := tmpl.ExecuteTemplate(w, name, data); err != nil {
			log.Printf("rendering %s: %v", name, err)
		}
	}

	var auth *githubAuth
	if cfg.enabled() {
		auth = newGitHubAuth(cfg, execute)
	}

	// lookup serves a cached report, or analyzes and caches a new one. Failures
	// are not cached, so a retry after a transient error fetches again.
	lookup := func(ctx context.Context, token, username string) (metrics.Report, error) {
		if !loginPattern.MatchString(username) {
			return metrics.Report{}, errInvalidLogin
		}
		if r, ok := cache.get(username); ok {
			return r, nil
		}
		if token == "" {
			return metrics.Report{}, errReadOnly
		}
		r, err := analyze(ctx, token, username)
		if err != nil {
			return metrics.Report{}, err
		}
		cache.add(r)
		return r, nil
	}

	mux := http.NewServeMux()
	if auth != nil {
		auth.register(mux)
	}
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		user, token := auth.current(r)
		username := r.FormValue("username")
		if username == "" {
			execute(w, "index.html", pageData{
				Reports: cache.list(), Version: version,
				ReadOnly: token == "", LoginEnabled: auth != nil, User: user,
			})
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
		if _, err := lookup(r.Context(), token, username); err != nil {
			log.Printf("analyzing %s: %v", username, err)
			execute(w, "analyze-error", pageData{Error: errorMessage(username, err)})
			return
		}
		w.Header().Set("HX-Redirect", target)
	})
	// Liveness only: it makes no GitHub call, so a GitHub outage does not get
	// the app restarted by whatever is probing it.
	// A missing OAuth setting is reported, not failed on: the app still serves
	// cached reports without it.
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(healthStatus{
			Status:                "ok",
			GitHubClientIDSet:     cfg.ClientID != "",
			GitHubClientSecretSet: cfg.ClientSecret != "",
			Missing:               cfg.missing(),
			AppBaseURL:            cfg.BaseURL,
		})
	})
	mux.HandleFunc("GET /debug", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		for _, row := range sysInfo() {
			_, _ = fmt.Fprintf(w, "%s: %s\n", row.Label, row.Value)
		}
	})
	mux.HandleFunc("GET /img/logo.svg", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/svg+xml")
		_, _ = w.Write(report.Logo)
	})
	mux.HandleFunc("GET /img/octopus-deploy-logo.png", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(ui.OctopusLogo)
	})
	mux.HandleFunc("GET /report/{username}", func(w http.ResponseWriter, r *http.Request) {
		username := r.PathValue("username")
		_, token := auth.current(r)
		rep, err := lookup(r.Context(), token, username)
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

// analyzeWithGitHub runs the same pipeline as the CLI with its default flags:
// the fetch with window fallback, then every metric. The token is the logged-in
// user's, so the analysis spends their rate limit.
func analyzeWithGitHub(ctx context.Context, token, username string) (metrics.Report, error) {
	client := analysis.NewClient(token)
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

func main() {
	// Without the OAuth app nobody can log in, and so nobody has a token to
	// analyze with: the app still starts, read-only, and serves what it has.
	cfg := authConfigFromEnv()
	if missing := cfg.missing(); len(missing) > 0 {
		log.Printf("%s not set: GitHub login is disabled, starting in read only mode", strings.Join(missing, " and "))
	}
	log.Printf("GitHub OAuth callback URL: %s/callback", cfg.BaseURL)
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           newHandler(analyzeWithGitHub, cfg),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("listening on %s", srv.Addr)
	log.Fatal(srv.ListenAndServe())
}
