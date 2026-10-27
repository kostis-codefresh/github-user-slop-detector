package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kostis-codefresh/github-user-slop-detector/pkg/analysis"
	"github.com/kostis-codefresh/github-user-slop-detector/pkg/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubAnalyzer stands in for GitHub: it counts calls per login, records the
// token each login was analyzed with, fails the logins listed in errs, gives the logins in needsReview a failed verdict, and
// stamps each report a minute after the previous one so the cache order is
// deterministic.
type stubAnalyzer struct {
	calls       map[string]int
	tokens      map[string]string
	errs        map[string]error
	needsReview map[string]bool
	clock       time.Time
}

func newStub() *stubAnalyzer {
	return &stubAnalyzer{
		calls:       make(map[string]int),
		tokens:      make(map[string]string),
		errs:        make(map[string]error),
		needsReview: make(map[string]bool),
		clock:       time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC),
	}
}

func (s *stubAnalyzer) analyze(_ context.Context, token, username string) (metrics.Report, error) {
	s.calls[username]++
	s.tokens[username] = token
	if err := s.errs[username]; err != nil {
		return metrics.Report{}, err
	}
	s.clock = s.clock.Add(time.Minute)
	return metrics.Report{
		Username: username, Passed: !s.needsReview[username], PassedCount: 5, TotalCount: 11, WindowEnd: s.clock,
	}, nil
}

// testAuthConfig is an OAuth app that is fully configured, so Login is offered.
var testAuthConfig = authConfig{
	ClientID: "test-client-id", ClientSecret: "test-client-secret",
	BaseURL: "http://localhost:8080", SessionKey: []byte("test-session-key"),
}

// sessionCookie is the session cookie of a user logged in as login with token,
// built through the same store the handler decodes it with.
func sessionCookie(t *testing.T, cfg authConfig, login, token string) *http.Cookie {
	t.Helper()
	store := newSessionStore(cfg)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	session, err := store.Get(req, sessionName)
	require.NoError(t, err, "a new session should not fail")
	session.Values["username"] = login
	session.Values["token"] = token
	require.NoError(t, session.Save(req, rec), "saving the session should succeed")
	cookies := rec.Result().Cookies()
	require.Len(t, cookies, 1, "saving the session should set one cookie")
	return cookies[0]
}

// withCookie sends cookie with every request to h.
func withCookie(h http.Handler, cookie *http.Cookie) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.AddCookie(cookie)
		h.ServeHTTP(w, r)
	})
}

// loggedInHandler is the app as seen by test-user, logged in with test-token.
func loggedInHandler(t *testing.T, stub *stubAnalyzer) http.Handler {
	t.Helper()
	return withCookie(newHandler(stub.analyze, testAuthConfig), sessionCookie(t, testAuthConfig, "test-user", "test-token"))
}

func get(t *testing.T, h http.Handler, path string, htmx bool) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if htmx {
		req.Header.Set("HX-Request", "true")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestReportPage(t *testing.T) {
	testCases := []struct {
		name         string
		path         string
		err          error
		expectedCode int
		expectedBody string
	}{
		{name: "Happy Path scenario", path: "/report/octocat", expectedCode: http.StatusOK, expectedBody: "@octocat"},
		{name: "Links back to the home page", path: "/report/octocat", expectedCode: http.StatusOK, expectedBody: `href="/"`},
		{name: "Rejects a login GitHub cannot have", path: "/report/no_such!", expectedCode: http.StatusNotFound, expectedBody: "not a valid GitHub username"},
		{
			name: "Unknown user is not found", path: "/report/ghost", err: fmt.Errorf("%w: ghost", analysis.ErrUserNotFound),
			expectedCode: http.StatusNotFound, expectedBody: "No GitHub user named ghost",
		},
		{
			name: "Failed fetch is a bad gateway", path: "/report/octocat", err: errors.New("GitHub rate limit exceeded"),
			expectedCode: http.StatusBadGateway, expectedBody: "GitHub rate limit exceeded",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			stub := newStub()
			stub.errs[strings.TrimPrefix(tc.path, "/report/")] = tc.err

			rec := get(t, loggedInHandler(t, stub), tc.path, false)

			assert.Equal(t, tc.expectedCode, rec.Code, "status code mismatch")
			assert.Contains(t, rec.Body.String(), tc.expectedBody, "body mismatch")
		})
	}
}

func TestReportsAreNotServedAtTheTopLevel(t *testing.T) {
	stub := newStub()

	rec := get(t, loggedInHandler(t, stub), "/octocat", false)

	assert.Equal(t, http.StatusNotFound, rec.Code, "reports live under /report/, so /octocat is not one")
	assert.Empty(t, stub.calls, "an unknown path should not cost an API call")
}

func TestReportPageRejectsInvalidLoginsWithoutCallingGitHub(t *testing.T) {
	stub := newStub()

	get(t, loggedInHandler(t, stub), "/report/no_such!", false)

	assert.Empty(t, stub.calls, "an impossible login should not cost an API call")
}

func TestReportPageServesRepeatsFromTheCache(t *testing.T) {
	stub := newStub()
	h := loggedInHandler(t, stub)

	first := get(t, h, "/report/octocat", false)
	second := get(t, h, "/report/OctoCat", false)

	require.Equal(t, http.StatusOK, first.Code, "the first request should render")
	assert.Equal(t, http.StatusOK, second.Code, "the cached request should render")
	assert.Equal(t, first.Body.String(), second.Body.String(), "a cached report renders exactly like a fresh one")
	assert.Equal(t, 1, stub.calls["octocat"], "logins are case-insensitive, so the second request is a cache hit")
}

func TestReportPageDoesNotCacheFailures(t *testing.T) {
	stub := newStub()
	stub.errs["octocat"] = errors.New("GitHub returned 502")
	h := loggedInHandler(t, stub)

	get(t, h, "/report/octocat", false)
	get(t, h, "/report/octocat", false)

	assert.Equal(t, 2, stub.calls["octocat"], "a failed fetch should be retried, not served from the cache")
}

func TestAnalyzeForm(t *testing.T) {
	testCases := []struct {
		name             string
		htmx             bool
		username         string
		err              error
		expectedCode     int
		expectedRedirect string
		expectedLocation string
		expectedBody     string
		expectedCalls    int
	}{
		{
			name: "Happy Path scenario", htmx: true, username: "octocat",
			expectedCode: http.StatusOK, expectedRedirect: "/report/octocat", expectedCalls: 1,
		},
		{
			name: "Unknown user stays on the form", htmx: true, username: "ghost",
			err:          fmt.Errorf("%w: ghost", analysis.ErrUserNotFound),
			expectedCode: http.StatusOK, expectedBody: "No GitHub user named ghost", expectedCalls: 1,
		},
		{
			name: "Invalid login stays on the form", htmx: true, username: "no_such!",
			expectedCode: http.StatusOK, expectedBody: "not a valid GitHub username",
		},
		{
			name: "Without htmx the form redirects to the report", username: "octocat",
			expectedCode: http.StatusSeeOther, expectedLocation: "/report/octocat",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			stub := newStub()
			stub.errs[tc.username] = tc.err

			rec := get(t, loggedInHandler(t, stub), "/?username="+tc.username, tc.htmx)

			assert.Equal(t, tc.expectedCode, rec.Code, "status code mismatch")
			assert.Equal(t, tc.expectedRedirect, rec.Header().Get("HX-Redirect"), "htmx redirect mismatch")
			assert.Equal(t, tc.expectedLocation, rec.Header().Get("Location"), "redirect mismatch")
			assert.Contains(t, rec.Body.String(), tc.expectedBody, "body mismatch")
			assert.Equal(t, tc.expectedCalls, stub.calls[tc.username], "analyze call count mismatch")
		})
	}
}

func TestAnalyzeFormLeavesTheReportInTheCache(t *testing.T) {
	stub := newStub()
	h := loggedInHandler(t, stub)

	get(t, h, "/?username=octocat", true)
	rec := get(t, h, "/report/octocat", false)

	assert.Equal(t, http.StatusOK, rec.Code, "the redirect target should render")
	assert.Equal(t, 1, stub.calls["octocat"], "the redirect target should be a cache hit")
}

func TestHomePage(t *testing.T) {
	testCases := []struct {
		name        string
		analyzed    []string
		needsReview string
		expected    []string
	}{
		{name: "Empty cache", expected: []string{"No reports yet."}},
		{
			name:     "Lists cached reports newest first",
			analyzed: []string{"first-user", "second-user"},
			expected: []string{`<a href="/report/second-user">second-user</a>`, "14 Oct 2026", `<a href="/report/first-user">first-user</a>`},
		},
		{
			name:        "Shows each report's points and result",
			analyzed:    []string{"first-user", "second-user"},
			needsReview: "first-user",
			expected:    []string{"second-user", "5/11", "Passed", "first-user", "5/11", "Needs Review"},
		},
		{
			name:     "Shows the version after the reports",
			analyzed: []string{"first-user"},
			expected: []string{"first-user", "<footer>Version dev</footer>"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			stub := newStub()
			stub.needsReview[tc.needsReview] = true
			h := loggedInHandler(t, stub)
			for _, login := range tc.analyzed {
				require.Equal(t, http.StatusOK, get(t, h, "/report/"+login, false).Code, "analyzing %s should succeed", login)
			}

			rec := get(t, h, "/", false)

			require.Equal(t, http.StatusOK, rec.Code, "the home page should render")
			body := rec.Body.String()
			at := 0
			for _, want := range tc.expected {
				i := strings.Index(body[at:], want)
				require.GreaterOrEqual(t, i, 0, "%q missing or out of order", want)
				at += i + len(want)
			}
			assert.NotContains(t, body, "{{", "the template should be fully rendered")
			assert.Contains(t, body, "Logged in as <strong>test-user</strong>", "a logged-in user should be named in the banner")
			assert.Contains(t, body, `<form class="logout" action="/logout" method="post">`, "logout should be a POST form")
			assert.Contains(t, body, "Logout</button>", "a logged-in user should be offered Logout")
			assert.NotContains(t, body, "Read only mode", "a logged-in user should see no read-only banner")
			assert.NotContains(t, body, "Login</a>", "a logged-in user should see no Login link")
		})
	}
}

func TestReadOnlyMode(t *testing.T) {
	const message = "Read only mode. Login with GitHub to start analyzing usernames"
	testCases := []struct {
		name         string
		path         string
		htmx         bool
		expectedCode int
		expected     []string
	}{
		{
			name:         "Home page shows the banner and disables Analyze",
			path:         "/",
			expectedCode: http.StatusOK,
			expected: []string{
				`<div class="banner warning">Read only mode.`, `<a class="github-button" href="/login">`, "Login</a>",
				"to start analyzing usernames</div>", `<button type="submit" disabled>Analyze</button>`,
			},
		},
		{
			name:         "Report page refuses to analyze",
			path:         "/report/octocat",
			expectedCode: http.StatusForbidden,
			expected:     []string{message},
		},
		{
			name:         "Form shows the read-only error",
			path:         "/?username=octocat",
			htmx:         true,
			expectedCode: http.StatusOK,
			expected:     []string{`<p class="status error">` + message},
		},
		{
			name:         "Invalid login is still a 404",
			path:         "/report/no_such!",
			expectedCode: http.StatusNotFound,
			expected:     []string{"not a valid GitHub username"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			stub := newStub()
			rec := get(t, newHandler(stub.analyze, testAuthConfig), tc.path, tc.htmx)

			assert.Equal(t, tc.expectedCode, rec.Code, "unexpected status for %s", tc.path)
			for _, want := range tc.expected {
				assert.Contains(t, rec.Body.String(), want, "the response for %s should contain %q", tc.path, want)
			}
			assert.NotContains(t, rec.Body.String(), "Logged in as", "a visitor who is not logged in has no user")
			assert.Empty(t, stub.calls, "a visitor who is not logged in should not cost an API call")
		})
	}
}

func TestOAuthNotConfigured(t *testing.T) {
	testCases := []struct {
		name string
		cfg  authConfig
	}{
		{name: "Neither client setting", cfg: authConfig{SessionKey: []byte("k")}},
		{name: "Only the client ID", cfg: authConfig{ClientID: "id", SessionKey: []byte("k")}},
		{name: "Only the client secret", cfg: authConfig{ClientSecret: "secret", SessionKey: []byte("k")}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHandler(newStub().analyze, tc.cfg)

			body := get(t, h, "/", false).Body.String()
			assert.Contains(t, body, `<div class="banner error">GitHub oauth client settings missing.</div>`, "the red banner should show")
			assert.NotContains(t, body, "Login", "without OAuth settings there should be no Login anywhere")
			assert.NotContains(t, body, "Read only mode", "the red banner replaces the read-only one")
			assert.Contains(t, body, `<button type="submit" disabled>Analyze</button>`, "Analyze should be disabled")
			assert.Equal(t, http.StatusNotFound, get(t, h, "/login", false).Code, "the login page should not exist")
		})
	}
}

func TestOAuthNotConfiguredIgnoresASessionCookie(t *testing.T) {
	stub := newStub()
	cfg := authConfig{SessionKey: testAuthConfig.SessionKey}
	h := withCookie(newHandler(stub.analyze, cfg), sessionCookie(t, cfg, "test-user", "test-token"))

	rec := get(t, h, "/report/octocat", false)

	assert.Equal(t, http.StatusForbidden, rec.Code, "without OAuth nobody is logged in, whatever their cookie says")
	assert.Empty(t, stub.calls, "nothing should be analyzed")
}

func TestAnalysisUsesTheLoggedInUsersToken(t *testing.T) {
	stub := newStub()

	get(t, loggedInHandler(t, stub), "/report/octocat", false)

	assert.Equal(t, "test-token", stub.tokens["octocat"], "the analysis should run on the logged-in user's token")
}

func TestCacheEvictsTheOldestReport(t *testing.T) {
	stub := newStub()
	h := loggedInHandler(t, stub)

	for i := range maxReports + 1 {
		get(t, h, fmt.Sprintf("/report/user-%d", i), false)
	}
	home := get(t, h, "/", false).Body.String()
	get(t, h, "/report/user-0", false)

	assert.NotContains(t, home, `href="/report/user-0"`, "the oldest report should be evicted")
	assert.Contains(t, home, `href="/report/user-1"`, "the next oldest report should survive")
	assert.Contains(t, home, fmt.Sprintf(`href="/report/user-%d"`, maxReports), "the newest report should be listed")
	assert.Equal(t, 2, stub.calls["user-0"], "an evicted report is fetched again")
}

func TestLogo(t *testing.T) {
	rec := get(t, newHandler(newStub().analyze, testAuthConfig), "/img/logo.svg", false)

	assert.Equal(t, http.StatusOK, rec.Code, "the logo should be served")
	assert.Equal(t, "image/svg+xml", rec.Header().Get("Content-Type"), "content type mismatch")
	assert.Contains(t, rec.Body.String(), "<svg", "body should be the SVG")
}

func TestHealth(t *testing.T) {
	testCases := []struct {
		name            string
		cfg             authConfig
		expectedID      bool
		expectedSecret  bool
		expectedMissing []string
	}{
		{name: "Both client settings", cfg: authConfig{ClientID: "id", ClientSecret: "secret"}, expectedID: true, expectedSecret: true, expectedMissing: []string{}},
		{name: "Only the client ID", cfg: authConfig{ClientID: "id"}, expectedID: true, expectedMissing: []string{"GITHUB_CLIENT_SECRET"}},
		{name: "Only the client secret", cfg: authConfig{ClientSecret: "secret"}, expectedSecret: true, expectedMissing: []string{"GITHUB_CLIENT_ID"}},
		{name: "Neither client setting", expectedMissing: []string{"GITHUB_CLIENT_ID", "GITHUB_CLIENT_SECRET"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.cfg.BaseURL = "https://reports.example.com"
			tc.cfg.SessionKey = []byte("k")

			rec := get(t, newHandler(newStub().analyze, tc.cfg), "/health", false)

			require.Equal(t, http.StatusOK, rec.Code, "the health check should succeed whatever is configured")
			assert.Equal(t, "application/json", rec.Header().Get("Content-Type"), "content type mismatch")
			var body healthStatus
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body), "body should be JSON")
			assert.Equal(t, "ok", body.Status, "status mismatch")
			assert.Equal(t, tc.expectedID, body.GitHubClientIDSet, "github_client_id_set mismatch")
			assert.Equal(t, tc.expectedSecret, body.GitHubClientSecretSet, "github_client_secret_set mismatch")
			assert.Equal(t, tc.expectedMissing, body.Missing, "missing mismatch")
			assert.Equal(t, "https://reports.example.com", body.AppBaseURL, "app_base_url mismatch")
		})
	}
}

func TestDebug(t *testing.T) {
	const storageVar = "WEBSITES_ENABLE_APP_SERVICE_STORAGE"
	testCases := []struct {
		name        string
		storage     string
		storageSet  bool
		expectedRow string
	}{
		{name: "Happy Path scenario", storage: "true", storageSet: true, expectedRow: "\n" + storageVar + ": true\n"},
		{name: "Storage setting false", storage: "false", storageSet: true, expectedRow: "\n" + storageVar + ": false\n"},
		{name: "Storage setting absent"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// t.Setenv first so the variable is restored afterwards, even when unset.
			t.Setenv(storageVar, tc.storage)
			if !tc.storageSet {
				require.NoError(t, os.Unsetenv(storageVar), "unsetting the storage variable should succeed")
			}

			rec := get(t, newHandler(newStub().analyze, testAuthConfig), "/debug", false)

			require.Equal(t, http.StatusOK, rec.Code, "the debug page should be served")
			assert.Equal(t, "text/plain; charset=utf-8", rec.Header().Get("Content-Type"), "content type mismatch")
			body := rec.Body.String()
			assert.Contains(t, body, "Go version: "+runtime.Version()+"\n", "body should carry the Go version line")
			assert.Contains(t, body, "\nGoroutines: ", "body should carry the goroutine count line")
			if tc.storageSet {
				assert.Contains(t, body, tc.expectedRow, "body should carry the storage setting line")
			} else {
				assert.NotContains(t, body, storageVar, "body should omit the storage setting when it is unset")
			}
		})
	}
}

func TestFormatBytes(t *testing.T) {
	assert.Equal(t, "512 B", formatBytes(512), "bytes mismatch")
	assert.Equal(t, "1.5 KiB", formatBytes(1536), "KiB mismatch")
	assert.Equal(t, "2.0 MiB", formatBytes(2<<20), "MiB mismatch")
	assert.Equal(t, "3.0 GiB", formatBytes(3<<30), "GiB mismatch")
}
