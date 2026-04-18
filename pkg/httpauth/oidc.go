package httpauth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/oauth2"
)

const (
	sessionCookieName = "golink_session"
	sessionDuration   = 24 * time.Hour
	stateCookieName   = "golink_oauth_state"
	loginPath         = "/auth/login"
	callbackPath      = "/auth/callback"
	logoutPath        = "/auth/logout"
)

// oidcHandler handles the OIDC login, callback, and logout HTTP routes.
type oidcHandler struct {
	cfg      AuthConfig
	provider *gooidc.Provider
	oauth2   oauth2.Config
	verifier *gooidc.IDTokenVerifier
	// canonicalAddr and jwtSecret are copied from the manager at construction.
	canonicalAddr string
	jwtSecret     string
	onAuth        func(email, name, avatarURL string)
	logger        *slog.Logger
}

func newOIDCHandler(ctx context.Context, m *AuthManager) (*oidcHandler, error) {
	provider, err := gooidc.NewProvider(ctx, m.cfg.OIDC.Issuer)
	if err != nil {
		return nil, fmt.Errorf("oidc provider: %w", err)
	}

	oauth2Cfg := oauth2.Config{
		ClientID:     m.cfg.OIDC.ClientID,
		ClientSecret: m.cfg.OIDC.ClientSecret,
		RedirectURL:  m.canonicalAddr + callbackPath,
		Endpoint:     provider.Endpoint(),
		Scopes:       m.cfg.OIDC.Scopes,
	}

	verifier := provider.Verifier(&gooidc.Config{ClientID: m.cfg.OIDC.ClientID})

	return &oidcHandler{
		cfg:           m.cfg,
		provider:      provider,
		oauth2:        oauth2Cfg,
		verifier:      verifier,
		canonicalAddr: m.canonicalAddr,
		jwtSecret:     m.jwtSecret,
		onAuth:        m.onAuth,
		logger:        m.logger,
	}, nil
}

// handleLogin redirects to the OIDC provider.
func (h *oidcHandler) handleLogin(w http.ResponseWriter, r *http.Request) {
	random, err := randomState()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Encode the post-login destination into the state so it survives the
	// round-trip to the OIDC provider. Format: "<random>|<rd>".
	rd := r.URL.Query().Get("rd")
	if rd == "" || !strings.HasPrefix(rd, "/") || strings.HasPrefix(rd, "//") {
		rd = "/"
	}
	state := random + "|" + rd

	http.SetCookie(w, &http.Cookie{
		Name:     stateCookieName,
		Value:    state,
		Path:     "/",
		MaxAge:   300,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, h.oauth2.AuthCodeURL(state), http.StatusFound)
}

// handleCallback processes the OIDC callback, validates the ID token, issues a
// session JWT cookie, and redirects to the original destination.
func (h *oidcHandler) handleCallback(w http.ResponseWriter, r *http.Request) {
	stateCookie, err := r.Cookie(stateCookieName)
	if err != nil || stateCookie.Value != r.URL.Query().Get("state") {
		http.Error(w, "invalid state", http.StatusBadRequest)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: stateCookieName, MaxAge: -1, Path: "/"})

	token, err := h.oauth2.Exchange(r.Context(), r.URL.Query().Get("code"))
	if err != nil {
		http.Error(w, "token exchange failed", http.StatusInternalServerError)
		return
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		http.Error(w, "missing id_token", http.StatusInternalServerError)
		return
	}

	idToken, err := h.verifier.Verify(r.Context(), rawIDToken)
	if err != nil {
		http.Error(w, "id_token verification failed", http.StatusUnauthorized)
		return
	}

	var claims struct {
		Email   string `json:"email"`
		Name    string `json:"name"`
		Picture string `json:"picture"`
	}
	if err := idToken.Claims(&claims); err != nil {
		http.Error(w, "claims extraction failed", http.StatusInternalServerError)
		return
	}

	var groups []string
	var rawClaims map[string]json.RawMessage
	if err := idToken.Claims(&rawClaims); err == nil {
		if gc, ok := rawClaims[h.cfg.OIDC.GroupsClaim]; ok {
			_ = json.Unmarshal(gc, &groups)
		}
	}

	id := &Identity{
		Email:       claims.Email,
		DisplayName: claims.Name,
		AvatarURL:   claims.Picture,
		Groups:      groups,
		Source:      AuthSourceOIDC,
	}
	id.IsAdmin = isAdmin(h.cfg, id)

	if h.onAuth != nil {
		go h.onAuth(id.Email, id.DisplayName, id.AvatarURL)
	}

	if err := h.issueSessionCookie(w, id); err != nil {
		http.Error(w, "session creation failed", http.StatusInternalServerError)
		return
	}

	dest := "/"
	if parts := strings.SplitN(stateCookie.Value, "|", 2); len(parts) == 2 {
		if rd := parts[1]; strings.HasPrefix(rd, "/") && !strings.HasPrefix(rd, "//") {
			dest = rd
		}
	}
	http.Redirect(w, r, dest, http.StatusFound)
}

// handleLogout clears the session cookie and redirects to home.
func (h *oidcHandler) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/", http.StatusFound)
}

// sessionClaims is the JWT payload stored in the session cookie.
type sessionClaims struct {
	jwt.RegisteredClaims
	Email       string   `json:"email"`
	DisplayName string   `json:"name"`
	AvatarURL   string   `json:"picture"`
	Groups      []string `json:"groups"`
}

func (h *oidcHandler) issueSessionCookie(w http.ResponseWriter, id *Identity) error {
	claims := sessionClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(sessionDuration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
		Email:       id.Email,
		DisplayName: id.DisplayName,
		AvatarURL:   id.AvatarURL,
		Groups:      id.Groups,
	}
	jwtToken := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := jwtToken.SignedString([]byte(h.jwtSecret))
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    signed,
		Path:     "/",
		MaxAge:   int(sessionDuration.Seconds()),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

// oidcMiddleware reads the session JWT cookie and populates the identity
// context. If OIDC is disabled or no valid cookie is present, the request
// passes through unchanged.
func (m *AuthManager) oidcMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if m.oidcH == nil {
				next.ServeHTTP(w, r)
				return
			}
			cookie, err := r.Cookie(sessionCookieName)
			if err != nil {
				m.logger.DebugContext(r.Context(), "oidc: no session cookie present")
				next.ServeHTTP(w, r)
				return
			}
			var claims sessionClaims
			jwtToken, err := jwt.ParseWithClaims(cookie.Value, &claims, func(t *jwt.Token) (any, error) {
				if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
				}
				return []byte(m.jwtSecret), nil
			})
			if err != nil || !jwtToken.Valid {
				m.logger.DebugContext(r.Context(), "oidc: invalid or expired session cookie", "error", err)
				next.ServeHTTP(w, r)
				return
			}
			id := &Identity{
				Email:       claims.Email,
				DisplayName: claims.DisplayName,
				AvatarURL:   claims.AvatarURL,
				Groups:      claims.Groups,
				Source:      AuthSourceOIDC,
			}
			id.IsAdmin = isAdmin(m.cfg, id)
			m.logger.DebugContext(r.Context(), "oidc: identity established",
				"email", id.Email,
				"is_admin", id.IsAdmin,
			)
			next.ServeHTTP(w, r.WithContext(WithIdentity(r.Context(), id)))
		})
	}
}

func randomState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}
