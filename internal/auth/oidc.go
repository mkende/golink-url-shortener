package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v5"
	"github.com/mkende/golink-redirector/internal/config"
	"github.com/mkende/golink-redirector/internal/db"
	"golang.org/x/oauth2"
)

const (
	sessionCookieName = "golink_session"
	sessionDuration   = 24 * time.Hour
	stateCookieName   = "golink_oauth_state"
)

// OIDCHandler handles OIDC login, callback, and logout routes.
type OIDCHandler struct {
	cfg      *config.Config
	provider *gooidc.Provider
	oauth2   oauth2.Config
	verifier *gooidc.IDTokenVerifier
	users    db.UserRepo
}

// NewOIDCHandler creates a new OIDCHandler. Returns an error if the OIDC
// provider cannot be reached. The users repo may be nil; if provided, users are
// upserted asynchronously on each successful authentication.
func NewOIDCHandler(ctx context.Context, cfg *config.Config, users db.UserRepo) (*OIDCHandler, error) {
	provider, err := gooidc.NewProvider(ctx, cfg.OIDC.Issuer)
	if err != nil {
		return nil, fmt.Errorf("oidc provider: %w", err)
	}

	oauth2Cfg := oauth2.Config{
		ClientID:     cfg.OIDC.ClientID,
		ClientSecret: cfg.OIDC.ClientSecret,
		RedirectURL:  cfg.OIDC.RedirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       cfg.OIDC.Scopes,
	}

	verifier := provider.Verifier(&gooidc.Config{ClientID: cfg.OIDC.ClientID})

	return &OIDCHandler{
		cfg:      cfg,
		provider: provider,
		oauth2:   oauth2Cfg,
		verifier: verifier,
		users:    users,
	}, nil
}

// HandleLogin redirects to the OIDC provider.
func (h *OIDCHandler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	state, err := randomState()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
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

// HandleCallback processes the OIDC callback, validates the ID token, issues a
// session JWT cookie, and redirects to the original destination.
func (h *OIDCHandler) HandleCallback(w http.ResponseWriter, r *http.Request) {
	// Validate state
	stateCookie, err := r.Cookie(stateCookieName)
	if err != nil || stateCookie.Value != r.URL.Query().Get("state") {
		http.Error(w, "invalid state", http.StatusBadRequest)
		return
	}
	// Clear state cookie
	http.SetCookie(w, &http.Cookie{Name: stateCookieName, MaxAge: -1, Path: "/"})

	// Exchange code
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

	// Extract standard claims
	var claims struct {
		Email   string `json:"email"`
		Name    string `json:"name"`
		Picture string `json:"picture"`
	}
	if err := idToken.Claims(&claims); err != nil {
		http.Error(w, "claims extraction failed", http.StatusInternalServerError)
		return
	}

	// Extract groups from the configured claim name
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
	}
	id.IsAdmin = isAdmin(h.cfg, id)

	if h.users != nil {
		go func() {
			if _, err := h.users.Upsert(context.Background(), id.Email, id.DisplayName, id.AvatarURL); err != nil {
				_ = err
			}
		}()
	}

	// Issue JWT session cookie
	if err := h.issueSessionCookie(w, id); err != nil {
		http.Error(w, "session creation failed", http.StatusInternalServerError)
		return
	}

	// Redirect to original destination or home
	dest := "/"
	if rd := r.URL.Query().Get("rd"); rd != "" && rd[0] == '/' {
		dest = rd
	}
	http.Redirect(w, r, dest, http.StatusFound)
}

// HandleLogout clears the session cookie and redirects to home.
func (h *OIDCHandler) HandleLogout(w http.ResponseWriter, r *http.Request) {
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
	IsAdmin     bool     `json:"is_admin"`
}

func (h *OIDCHandler) issueSessionCookie(w http.ResponseWriter, id *Identity) error {
	claims := sessionClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(sessionDuration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
		Email:       id.Email,
		DisplayName: id.DisplayName,
		AvatarURL:   id.AvatarURL,
		Groups:      id.Groups,
		IsAdmin:     id.IsAdmin,
	}
	jwtToken := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := jwtToken.SignedString([]byte(h.cfg.OIDC.JWTSecret))
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

// OIDCMiddleware reads the session JWT cookie and populates the identity
// context. If OIDC is disabled or no valid cookie is present, the request
// passes through unchanged.
func OIDCMiddleware(cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !cfg.OIDC.Enabled {
				next.ServeHTTP(w, r)
				return
			}
			cookie, err := r.Cookie(sessionCookieName)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}
			var claims sessionClaims
			jwtToken, err := jwt.ParseWithClaims(cookie.Value, &claims, func(t *jwt.Token) (interface{}, error) {
				if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
				}
				return []byte(cfg.OIDC.JWTSecret), nil
			})
			if err != nil || !jwtToken.Valid {
				next.ServeHTTP(w, r)
				return
			}
			id := &Identity{
				Email:       claims.Email,
				DisplayName: claims.DisplayName,
				AvatarURL:   claims.AvatarURL,
				Groups:      claims.Groups,
				IsAdmin:     claims.IsAdmin,
			}
			next.ServeHTTP(w, r.WithContext(WithIdentity(r.Context(), id)))
		})
	}
}

// randomState generates a random base64-encoded state string for OAuth2 CSRF
// protection.
func randomState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}
