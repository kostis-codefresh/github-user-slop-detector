package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// githubStub stands in for github.com and api.github.com: it hands out
// "oauth-token" for any code, answers /user and /user/emails with its fields,
// and records grant revocations.
type githubStub struct {
	*httptest.Server
	login   string
	emails  []githubEmail
	revoked []string // the access tokens whose grant was deleted
	auth    string   // the basic-auth user:password of the last revocation
}

func newGitHubStub(t *testing.T, login string, emails ...githubEmail) *githubStub {
	t.Helper()
	stub := &githubStub{login: login, emails: emails}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /login/oauth/access_token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"oauth-token","token_type":"bearer"}`))
	})
	requireToken := func(w http.ResponseWriter, r *http.Request) bool {
		if r.Header.Get("Authorization") != "Bearer oauth-token" {
			http.Error(w, "bad credentials", http.StatusUnauthorized)
			return false
		}
		return true
	}
	mux.HandleFunc("GET /user", func(w http.ResponseWriter, r *http.Request) {
		if requireToken(w, r) {
			_ = json.NewEncoder(w).Encode(githubUser{Login: stub.login})
		}
	})
	mux.HandleFunc("GET /user/emails", func(w http.ResponseWriter, r *http.Request) {
		if requireToken(w, r) {
			_ = json.NewEncoder(w).Encode(stub.emails)
		}
	})
	mux.HandleFunc("DELETE /applications/{client}/grant", func(w http.ResponseWriter, r *http.Request) {
		user, pass, _ := r.BasicAuth()
		stub.auth = user + ":" + pass
		var body struct {
			AccessToken string `json:"access_token"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		stub.revoked = append(stub.revoked, body.AccessToken)
		w.WriteHeader(http.StatusNoContent)
	})
	stub.Server = httptest.NewServer(mux)
	t.Cleanup(stub.Close)
	return stub
}

func (s *githubStub) config() authConfig {
	cfg := testAuthConfig
	cfg.githubURL = s.URL
	cfg.apiURL = s.URL
	return cfg
}

// login runs the whole flow against h: /login/github, then GitHub's redirect
// back to /callback with the state it was given. It returns the callback's
// response.
func login(t *testing.T, h http.Handler) *httptest.ResponseRecorder {
	t.Helper()
	start := get(t, h, "/login/github", false)
	require.Equal(t, http.StatusTemporaryRedirect, start.Code, "login should redirect to GitHub")
	authorize, err := url.Parse(start.Header().Get("Location"))
	require.NoError(t, err, "the redirect should be a URL")
	cookies := start.Result().Cookies()
	require.Len(t, cookies, 1, "login should set the session cookie holding the state")

	req := httptest.NewRequest(http.MethodGet, "/callback?code=the-code&state="+url.QueryEscape(authorize.Query().Get("state")), nil)
	req.AddCookie(cookies[0])
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestLoginPage(t *testing.T) {
	rec := get(t, newHandler(newStub().analyze, testAuthConfig, ""), "/login", false)

	assert.Equal(t, http.StatusOK, rec.Code, "the login page should render")
	for _, want := range []string{
		`src="/img/octopus-deploy-logo.png"`, "Octopus Employees only", `href="/login/github"`, "Login with GitHub",
	} {
		assert.Contains(t, rec.Body.String(), want, "the login page should contain %q", want)
	}
}

func TestOctopusLogo(t *testing.T) {
	rec := get(t, newHandler(newStub().analyze, testAuthConfig, ""), "/img/octopus-deploy-logo.png", false)

	assert.Equal(t, http.StatusOK, rec.Code, "the logo should be served")
	assert.Equal(t, "image/png", rec.Header().Get("Content-Type"), "content type mismatch")
	assert.True(t, strings.HasPrefix(rec.Body.String(), "\x89PNG"), "body should be the PNG")
}

func TestLoginRedirectsToGitHub(t *testing.T) {
	rec := get(t, newHandler(newStub().analyze, testAuthConfig, ""), "/login/github", false)

	require.Equal(t, http.StatusTemporaryRedirect, rec.Code, "login should redirect")
	location, err := url.Parse(rec.Header().Get("Location"))
	require.NoError(t, err, "the redirect should be a URL")
	assert.Equal(t, "https://github.com/login/oauth/authorize", location.Scheme+"://"+location.Host+location.Path, "should redirect to GitHub's authorize page")
	q := location.Query()
	assert.Equal(t, "test-client-id", q.Get("client_id"), "client_id mismatch")
	assert.Equal(t, "http://localhost:8080/callback", q.Get("redirect_uri"), "redirect_uri should be APP_BASE_URL/callback")
	assert.Equal(t, "read:user user:email", q.Get("scope"), "scope mismatch")
	assert.NotEmpty(t, q.Get("state"), "a state should be sent")
}

func TestCallback(t *testing.T) {
	testCases := []struct {
		name         string
		emails       []githubEmail
		expectedCode int
		expectedBody string
		loggedIn     bool
	}{
		{
			name:         "Happy Path scenario",
			emails:       []githubEmail{{Email: "Jane@Octopus.com", Primary: true, Verified: true}},
			expectedCode: http.StatusSeeOther, loggedIn: true,
		},
		{
			name: "Accepts a verified octopus secondary email",
			emails: []githubEmail{
				{Email: "jane@example.com", Primary: true, Verified: true},
				{Email: "jane@octopus.com", Verified: true},
			},
			expectedCode: http.StatusSeeOther, loggedIn: true,
		},
		{
			name:         "Problematic scenario",
			emails:       []githubEmail{{Email: "jane@example.com", Primary: true, Verified: true}},
			expectedCode: http.StatusForbidden, expectedBody: "Access Denied: None of your verified emails (jane@example.com) is a @octopus.com address.",
		},
		{
			name: "An unverified octopus secondary does not count",
			emails: []githubEmail{
				{Email: "jane@example.com", Primary: true, Verified: true},
				{Email: "jane@octopus.com"},
			},
			expectedCode: http.StatusForbidden, expectedBody: "Access Denied: None of your verified emails (jane@example.com) is a @octopus.com address.",
		},
		{
			name:         "An unverified octopus email does not count",
			emails:       []githubEmail{{Email: "jane@octopus.com", Primary: true}},
			expectedCode: http.StatusBadGateway, expectedBody: "no verified email found",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gh := newGitHubStub(t, "jane", tc.emails...)
			h := newHandler(newStub().analyze, gh.config(), "")

			rec := login(t, h)

			assert.Equal(t, tc.expectedCode, rec.Code, "callback status mismatch")
			assert.Contains(t, rec.Body.String(), tc.expectedBody, "callback body mismatch")
			home := get(t, withCookie(h, rec.Result().Cookies()[0]), "/", false).Body.String()
			if tc.loggedIn {
				assert.Equal(t, "/", rec.Header().Get("Location"), "a login should land on the home page")
				assert.Contains(t, home, "Logged in as <strong>jane</strong>", "the banner should name the GitHub login")
			} else {
				assert.Contains(t, home, "Read only mode", "a denied user should not be logged in")
			}
		})
	}
}

func TestCallbackRejectsABadState(t *testing.T) {
	testCases := []struct {
		name  string
		state string
	}{
		{name: "Missing state", state: ""},
		{name: "Forged state", state: "not-the-state"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gh := newGitHubStub(t, "jane", githubEmail{Email: "jane@octopus.com", Primary: true, Verified: true})
			h := newHandler(newStub().analyze, gh.config(), "")
			start := get(t, h, "/login/github", false)

			req := httptest.NewRequest(http.MethodGet, "/callback?code=the-code&state="+tc.state, nil)
			req.AddCookie(start.Result().Cookies()[0])
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusForbidden, rec.Code, "a bad state should be refused")
			assert.Contains(t, rec.Body.String(), "Invalid or missing state parameter", "body mismatch")
		})
	}
}

func TestLogout(t *testing.T) {
	gh := newGitHubStub(t, "jane", githubEmail{Email: "jane@octopus.com", Primary: true, Verified: true})
	h := newHandler(newStub().analyze, gh.config(), "")
	session := login(t, h).Result().Cookies()[0]

	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.AddCookie(session)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusSeeOther, rec.Code, "logout should redirect")
	assert.Equal(t, "/", rec.Header().Get("Location"), "logout should land on the home page")
	assert.Equal(t, []string{"oauth-token"}, gh.revoked, "logout should revoke the user's grant on GitHub")
	assert.Equal(t, "test-client-id:test-client-secret", gh.auth, "the revocation authenticates as the OAuth app")
	cookies := rec.Result().Cookies()
	require.Len(t, cookies, 1, "logout should reset the session cookie")
	assert.Negative(t, cookies[0].MaxAge, "the session cookie should be deleted")
}

func TestLogoutIsNotAGet(t *testing.T) {
	gh := newGitHubStub(t, "jane")
	h := newHandler(newStub().analyze, gh.config(), "")

	rec := get(t, withCookie(h, sessionCookie(t, gh.config(), "jane", "oauth-token")), "/logout", false)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code, "a cross-site GET should not log anyone out")
	assert.Empty(t, gh.revoked, "nothing should be revoked")
}
