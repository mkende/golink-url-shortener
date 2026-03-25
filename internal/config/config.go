// Package config provides configuration loading and validation for golink-redirector.
package config

import (
	"errors"
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

// TailscaleConfig holds settings for Tailscale header-based authentication.
type TailscaleConfig struct {
	// Enabled controls whether Tailscale header-based auth is active.
	Enabled bool `toml:"enabled"`
}

// OIDCConfig holds settings for OpenID Connect authentication.
type OIDCConfig struct {
	// Enabled controls whether OIDC auth is active.
	Enabled bool `toml:"enabled"`
	// Issuer is the OIDC provider issuer URL (e.g. "https://accounts.google.com").
	Issuer string `toml:"issuer"`
	// ClientID is the OAuth2 client identifier.
	ClientID string `toml:"client_id"`
	// ClientSecret is the OAuth2 client secret.
	ClientSecret string `toml:"client_secret"`
	// RedirectURL is the OAuth2 redirect (callback) URL.
	RedirectURL string `toml:"redirect_url"`
	// Scopes is the list of OAuth2 scopes to request.
	// Defaults to ["openid", "email", "profile"].
	Scopes []string `toml:"scopes"`
	// GroupsClaim is the JWT claim name that contains group memberships.
	// Defaults to "groups".
	GroupsClaim string `toml:"groups_claim"`
	// JWTSecret is the HMAC secret used to sign and verify session JWT cookies.
	// Required when OIDC is enabled. Use a long random string (>= 32 bytes).
	JWTSecret string `toml:"jwt_secret"`
}

// DBConfig holds database connection settings.
type DBConfig struct {
	// Driver selects the database backend. Valid values: "sqlite", "postgres".
	// Defaults to "sqlite".
	Driver string `toml:"driver"`
	// DSN is the data source name / connection string.
	// For SQLite this is a file path; defaults to "golink.db".
	DSN string `toml:"dsn"`
}

// Config is the top-level application configuration.
// It is loaded from a TOML file by Load.
type Config struct {
	// ListenAddr is the TCP address the HTTP server binds to.
	// Defaults to "0.0.0.0:8080".
	ListenAddr string `toml:"listen_addr"`

	// CanonicalDomain is the public hostname used for HTTPS redirects and OIDC
	// callbacks (e.g. "go.example.com"). Required.
	CanonicalDomain string `toml:"canonical_domain"`

	// Title is the human-readable name shown in the UI.
	// Defaults to "GoLink".
	Title string `toml:"title"`

	// FaviconPath is a filesystem path to a custom favicon file.
	// An empty string means the embedded default favicon is used.
	FaviconPath string `toml:"favicon_path"`

	// Tailscale holds settings for Tailscale header-based authentication.
	Tailscale TailscaleConfig `toml:"tailscale"`

	// OIDC holds settings for OpenID Connect authentication.
	OIDC OIDCConfig `toml:"oidc"`

	// RequireAuthForRedirects controls whether unauthenticated users are
	// blocked from following any redirect.
	RequireAuthForRedirects bool `toml:"require_auth_for_redirects"`

	// DB holds database connection settings.
	DB DBConfig `toml:"db"`

	// QuickLinkLength is the number of characters in a randomly-generated
	// quick-link name. Must be >= 4. Defaults to 6.
	QuickLinkLength int `toml:"quick_link_length"`

	// DefaultDomain is appended to bare email addresses (without an @) when
	// resolving share targets.
	DefaultDomain string `toml:"default_domain"`

	// RequiredDomain restricts link sharing to a single email domain. An empty
	// string disables the restriction.
	RequiredDomain string `toml:"required_domain"`

	// AdminEmails lists the email addresses of users with admin privileges.
	AdminEmails []string `toml:"admin_emails"`

	// AdminGroup is the OIDC group name whose members are treated as admins.
	AdminGroup string `toml:"admin_group"`

	// CacheSize is the maximum number of links kept in the in-process LRU
	// redirect cache. Increasing this reduces database reads on the hot path.
	// Defaults to 1000.
	CacheSize int `toml:"cache_size"`
}

// applyDefaults fills in zero-value fields with their documented defaults.
func applyDefaults(c *Config) {
	if c.ListenAddr == "" {
		c.ListenAddr = "0.0.0.0:8080"
	}
	if c.Title == "" {
		c.Title = "GoLink"
	}
	if len(c.OIDC.Scopes) == 0 {
		c.OIDC.Scopes = []string{"openid", "email", "profile"}
	}
	if c.OIDC.GroupsClaim == "" {
		c.OIDC.GroupsClaim = "groups"
	}
	if c.DB.Driver == "" {
		c.DB.Driver = "sqlite"
	}
	if c.DB.DSN == "" {
		c.DB.DSN = "golink.db"
	}
	if c.QuickLinkLength == 0 {
		c.QuickLinkLength = 6
	}
	if c.CacheSize == 0 {
		c.CacheSize = 1000
	}
}

// validate checks that required fields are present and values are within
// acceptable ranges. It returns a descriptive error for the first violation
// found.
func validate(c *Config) error {
	if c.CanonicalDomain == "" {
		return errors.New("canonical_domain is required")
	}
	if c.QuickLinkLength < 4 {
		return fmt.Errorf("quick_link_length must be >= 4, got %d", c.QuickLinkLength)
	}
	switch c.DB.Driver {
	case "sqlite", "postgres":
		// valid
	default:
		return fmt.Errorf("db.driver must be \"sqlite\" or \"postgres\", got %q", c.DB.Driver)
	}
	if c.OIDC.Enabled && c.OIDC.JWTSecret == "" {
		return errors.New("oidc.jwt_secret is required when OIDC is enabled")
	}
	return nil
}

// Load reads the TOML configuration file at path, applies defaults for any
// omitted fields, and validates the result. It returns a fully populated
// *Config or a descriptive error.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file %q: %w", path, err)
	}

	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file %q: %w", path, err)
	}

	applyDefaults(&cfg)

	if err := validate(&cfg); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &cfg, nil
}
