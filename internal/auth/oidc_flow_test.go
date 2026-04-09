package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/mkende/golink-url-shortener/internal/auth"
	"github.com/mkende/golink-url-shortener/internal/config"
	"github.com/oauth2-proxy/mockoidc"
)

// newOIDCHandlerForTest creates an OIDCHandler backed by the given mock
// OIDC server. CanonicalAddress is set to a placeholder; the mock token
// endpoint does not validate redirect_uri so this is fine in tests.
func newOIDCHandlerForTest(t *testing.T, m *mockoidc.MockOIDC) *auth.OIDCHandler {
	t.Helper()
	cfg := &config.Config{
		CanonicalAddress: "https://go.example.com",
		JWTSecret: testJWTSecret,
		OIDC: config.OIDCConfig{
			Enabled:      true,
			Issuer:       m.Issuer(),
			ClientID:     m.ClientID,
			ClientSecret: m.ClientSecret,
			Scopes:       []string{"openid", "email"},
		},
	}
	h, err := auth.NewOIDCHandler(context.Background(), cfg, nil)
	if err != nil {
		t.Fatalf("NewOIDCHandler: %v", err)
	}
	return h
}

// TestHandleLogin_StateEncoding checks that the rd destination is embedded in
// the OAuth2 state cookie so it survives the round-trip to the OIDC provider.
func TestHandleLogin_StateEncoding(t *testing.T) {
	m, err := mockoidc.Run()
	if err != nil {
		t.Fatalf("start mock OIDC: %v", err)
	}
	defer m.Shutdown()

	h := newOIDCHandlerForTest(t, m)

	req := httptest.NewRequest(http.MethodGet, "/auth/login?rd=%2Fmy%2Flink", nil)
	rec := httptest.NewRecorder()
	h.HandleLogin(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusFound)
	}

	// State cookie must exist and carry the destination after the "|" separator.
	stateCookie := findCookie(rec.Result().Cookies(), "golink_oauth_state")
	if stateCookie == nil {
		t.Fatal("state cookie not set")
	}
	parts := strings.SplitN(stateCookie.Value, "|", 2)
	if len(parts) != 2 {
		t.Fatalf("state cookie %q: want format <random>|<rd>", stateCookie.Value)
	}
	if got, want := parts[1], "/my/link"; got != want {
		t.Errorf("rd in state: got %q, want %q", got, want)
	}

	// The redirect URL must point at the OIDC provider and include the full state.
	location := rec.Header().Get("Location")
	authBase := m.Addr() + mockoidc.AuthorizationEndpoint
	if !strings.HasPrefix(location, authBase) {
		t.Errorf("redirect %q does not start with %q", location, authBase)
	}
	u, _ := url.Parse(location)
	if got := u.Query().Get("state"); got != stateCookie.Value {
		t.Errorf("state in URL %q != state cookie %q", got, stateCookie.Value)
	}
}

// TestHandleLogin_UnsafeRdDefaultsToRoot verifies that missing or unsafe rd
// values are replaced by "/" before being embedded in the state.
func TestHandleLogin_UnsafeRdDefaultsToRoot(t *testing.T) {
	m, err := mockoidc.Run()
	if err != nil {
		t.Fatalf("start mock OIDC: %v", err)
	}
	defer m.Shutdown()

	h := newOIDCHandlerForTest(t, m)

	cases := []struct {
		name string
		rd   string
		want string
	}{
		{"empty", "", "/"},
		{"protocol-relative", "//evil.com", "/"},
		{"absolute URL", "https://evil.com/steal", "/"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := "/auth/login"
			if tc.rd != "" {
				path += "?rd=" + url.QueryEscape(tc.rd)
			}
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			h.HandleLogin(rec, req)

			stateCookie := findCookie(rec.Result().Cookies(), "golink_oauth_state")
			if stateCookie == nil {
				t.Fatal("state cookie not set")
			}
			parts := strings.SplitN(stateCookie.Value, "|", 2)
			if len(parts) != 2 {
				t.Fatalf("state %q: wrong format", stateCookie.Value)
			}
			if got := parts[1]; got != tc.want {
				t.Errorf("rd: got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestHandleCallback_RdPreserved exercises the full login → OIDC provider →
// callback chain and confirms that the post-login destination encoded in the
// state is used for the final redirect.
func TestHandleCallback_RdPreserved(t *testing.T) {
	m, err := mockoidc.Run()
	if err != nil {
		t.Fatalf("start mock OIDC: %v", err)
	}
	defer m.Shutdown()

	h := newOIDCHandlerForTest(t, m)

	// Step 1: HandleLogin — capture the state cookie and the URL that the
	// browser would be sent to.
	loginReq := httptest.NewRequest(http.MethodGet, "/auth/login?rd=%2Fmy%2Flink", nil)
	loginRec := httptest.NewRecorder()
	h.HandleLogin(loginRec, loginReq)
	if loginRec.Code != http.StatusFound {
		t.Fatalf("login status: got %d, want %d", loginRec.Code, http.StatusFound)
	}
	stateCookie := findCookie(loginRec.Result().Cookies(), "golink_oauth_state")
	if stateCookie == nil {
		t.Fatal("state cookie not set by HandleLogin")
	}
	authURL := loginRec.Header().Get("Location")

	// Step 2: Hit the OIDC provider's authorize endpoint. The mock server
	// redirects straight back to our redirect_uri with a code and the state.
	// Stop at the redirect so we can extract those values.
	noFollow := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	authResp, err := noFollow.Get(authURL)
	if err != nil {
		t.Fatalf("GET authorize: %v", err)
	}
	authResp.Body.Close()
	if authResp.StatusCode != http.StatusFound {
		t.Fatalf("authorize response: got %d, want 302", authResp.StatusCode)
	}
	callbackURL, err := url.Parse(authResp.Header.Get("Location"))
	if err != nil {
		t.Fatalf("parse callback location: %v", err)
	}
	code := callbackURL.Query().Get("code")
	state := callbackURL.Query().Get("state")
	if code == "" || state == "" {
		t.Fatalf("authorize redirect missing code or state: %s", callbackURL)
	}

	// Step 3: HandleCallback — present the state cookie from step 1 and the
	// code+state from step 2, as a real browser would.
	cbPath := "/auth/callback?code=" + url.QueryEscape(code) + "&state=" + url.QueryEscape(state)
	cbReq := httptest.NewRequest(http.MethodGet, cbPath, nil)
	cbReq.AddCookie(stateCookie)
	cbRec := httptest.NewRecorder()
	h.HandleCallback(cbRec, cbReq)

	if cbRec.Code != http.StatusFound {
		t.Fatalf("callback status: got %d, want %d", cbRec.Code, http.StatusFound)
	}
	if dest := cbRec.Header().Get("Location"); dest != "/my/link" {
		t.Errorf("post-login destination: got %q, want %q", dest, "/my/link")
	}
	if findCookie(cbRec.Result().Cookies(), "golink_session") == nil {
		t.Error("session cookie not set after successful callback")
	}
}

func findCookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, c := range cookies {
		if c.Name == name {
			return c
		}
	}
	return nil
}
