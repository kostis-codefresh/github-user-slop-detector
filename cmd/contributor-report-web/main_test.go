package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kostis-codefresh/github-user-slop-detector/pkg/analysis"
	"github.com/kostis-codefresh/github-user-slop-detector/pkg/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubAnalyzer stands in for GitHub: it counts calls per login, fails the
// logins listed in errs, gives the logins in needsReview a failed verdict, and
// stamps each report a minute after the previous one so the cache order is
// deterministic.
type stubAnalyzer struct {
	calls       map[string]int
	errs        map[string]error
	needsReview map[string]bool
	clock       time.Time
}

func newStub() *stubAnalyzer {
	return &stubAnalyzer{
		calls:       make(map[string]int),
		errs:        make(map[string]error),
		needsReview: make(map[string]bool),
		clock:       time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC),
	}
}

func (s *stubAnalyzer) analyze(_ context.Context, username string) (metrics.Report, error) {
	s.calls[username]++
	if err := s.errs[username]; err != nil {
		return metrics.Report{}, err
	}
	s.clock = s.clock.Add(time.Minute)
	return metrics.Report{
		Username: username, Passed: !s.needsReview[username], PassedCount: 5, TotalCount: 11, WindowEnd: s.clock,
	}, nil
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

			rec := get(t, newHandler(stub.analyze), tc.path, false)

			assert.Equal(t, tc.expectedCode, rec.Code, "status code mismatch")
			assert.Contains(t, rec.Body.String(), tc.expectedBody, "body mismatch")
		})
	}
}

func TestReportsAreNotServedAtTheTopLevel(t *testing.T) {
	stub := newStub()

	rec := get(t, newHandler(stub.analyze), "/octocat", false)

	assert.Equal(t, http.StatusNotFound, rec.Code, "reports live under /report/, so /octocat is not one")
	assert.Empty(t, stub.calls, "an unknown path should not cost an API call")
}

func TestReportPageRejectsInvalidLoginsWithoutCallingGitHub(t *testing.T) {
	stub := newStub()

	get(t, newHandler(stub.analyze), "/report/no_such!", false)

	assert.Empty(t, stub.calls, "an impossible login should not cost an API call")
}

func TestReportPageServesRepeatsFromTheCache(t *testing.T) {
	stub := newStub()
	h := newHandler(stub.analyze)

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
	h := newHandler(stub.analyze)

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

			rec := get(t, newHandler(stub.analyze), "/?username="+tc.username, tc.htmx)

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
	h := newHandler(stub.analyze)

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
			h := newHandler(stub.analyze)
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
		})
	}
}

func TestCacheEvictsTheOldestReport(t *testing.T) {
	stub := newStub()
	h := newHandler(stub.analyze)

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
	rec := get(t, newHandler(newStub().analyze), "/img/logo.svg", false)

	assert.Equal(t, http.StatusOK, rec.Code, "the logo should be served")
	assert.Equal(t, "image/svg+xml", rec.Header().Get("Content-Type"), "content type mismatch")
	assert.Contains(t, rec.Body.String(), "<svg", "body should be the SVG")
}

func TestHealth(t *testing.T) {
	rec := get(t, newHandler(newStub().analyze), "/health", false)

	require.Equal(t, http.StatusOK, rec.Code, "the health check should succeed")
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"), "content type mismatch")
	var body struct{ Status string }
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body), "body should be JSON")
	assert.Equal(t, "ok", body.Status, "status mismatch")
}
