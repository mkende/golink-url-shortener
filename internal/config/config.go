// Package config provides configuration loading and validation for golink-redirector.
package config

import (
	"errors"
	"fmt"
	"net"
	"os"

	"github.com/BurntSushi/toml"
)

// AnonymousConfig holds settings for anonymous (user-less) authentication.
// In this mode every request is automatically treated as a single shared
// anonymous user. Intended for local development, testing, or private
// instances where no real user management is needed.
// WARNING: Do not enable on a publicly reachable server.
type AnonymousConfig struct {
	// Enabled controls whether anonymous auth is active.
	Enabled bool `toml:"enabled"`
	// IsAdmin grants the anonymous user full admin privileges when true.
	// Useful for private/test instances where you also need API key management
	// and import/export access. Default: false.
	IsAdmin bool `toml:"is_admin"`
}

// TailscaleConfig holds settings for Tailscale header-based authentication.
type TailscaleConfig struct {
	// Enabled controls whether Tailscale header-based auth is active.
	Enabled bool `toml:"enabled"`
	// TrustedCIDRs is an optional list of IP ranges (IPv4 or IPv6 CIDR notation)
	// from which Tailscale-User-* headers are accepted. When the list is non-empty,
	// requests arriving from IPs outside these ranges have their Tailscale headers
	// silently ignored. An empty list preserves the original behaviour: headers
	// are trusted regardless of origin.
	TrustedCIDRs []string `toml:"trusted_cidrs"`
}

// ProxyAuthConfig holds settings for reverse-proxy header-based authentication.
// This is similar to Tailscale auth but reads generic forward-auth headers
// injected by a trusted reverse proxy such as nginx, Caddy, or Traefik.
// The header names default to the de-facto standard used by Authelia.
type ProxyAuthConfig struct {
	// Enabled controls whether proxy header-based auth is active.
	Enabled bool `toml:"enabled"`
	// TrustedCIDRs is the list of IP ranges from which proxy auth headers are
	// accepted. Required when enabled; requests from outside these ranges have
	// their headers silently ignored.
	TrustedCIDRs []string `toml:"trusted_cidrs"`
	// UserHeader is the header containing the authenticated user's login name
	// or username. Used as the primary identifier when EmailHeader is absent
	// or empty. Defaults to "Remote-User".
	UserHeader string `toml:"user_header"`
	// EmailHeader is the header containing the user's email address. When
	// present it is used as the primary user identifier (Identity.Email);
	// UserHeader is then treated as a supplementary username. Defaults to
	// "Remote-Email". Set to "" to disable and always use UserHeader instead.
	EmailHeader string `toml:"email_header"`
	// NameHeader is the header containing the user's display name.
	// Defaults to "Remote-Name".
	NameHeader string `toml:"name_header"`
	// GroupsHeader is the header containing a comma-separated list of the
	// user's group memberships. Defaults to "Remote-Groups".
	GroupsHeader string `toml:"groups_header"`
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

	// AllowHTTP disables the automatic HTTPS redirect for non-redirect requests.
	// When true, requests are served on whatever scheme they arrive on.
	// Defaults to false.
	AllowHTTP bool `toml:"allow_http"`

	// Anonymous holds settings for anonymous (user-less) authentication.
	Anonymous AnonymousConfig `toml:"anonymous"`

	// Tailscale holds settings for Tailscale header-based authentication.
	Tailscale TailscaleConfig `toml:"tailscale"`

	// ProxyAuth holds settings for reverse-proxy header-based authentication.
	ProxyAuth ProxyAuthConfig `toml:"proxy_auth"`

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

	// MaxAliasesPerLink is the maximum number of alias links that may target
	// any single canonical link.  Defaults to 100.
	MaxAliasesPerLink int `toml:"max_aliases_per_link"`

	// UI holds settings that control the behaviour of the web UI.
	UI UIConfig `toml:"ui"`
}

// UIConfig holds settings that control the behaviour of the web UI.
type UIConfig struct {
	// LinksPerPage is the number of links shown on each page of the /links and
	// /mylinks lists. Must be >= 10. Defaults to 100.
	LinksPerPage int `toml:"links_per_page"`
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
	if c.ProxyAuth.UserHeader == "" {
		c.ProxyAuth.UserHeader = "Remote-User"
	}
	if c.ProxyAuth.EmailHeader == "" {
		c.ProxyAuth.EmailHeader = "Remote-Email"
	}
	if c.ProxyAuth.NameHeader == "" {
		c.ProxyAuth.NameHeader = "Remote-Name"
	}
	if c.ProxyAuth.GroupsHeader == "" {
		c.ProxyAuth.GroupsHeader = "Remote-Groups"
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
	if c.MaxAliasesPerLink == 0 {
		c.MaxAliasesPerLink = 100
	}
	if c.UI.LinksPerPage == 0 {
		c.UI.LinksPerPage = 100
	}
}

// validateCIDRList returns an error if any entry in cidrs is not a valid CIDR,
// using fieldName in the error message for clarity.
func validateCIDRList(fieldName string, cidrs []string) error {
	for _, cidr := range cidrs {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return fmt.Errorf("%s: %q is not a valid CIDR: %w", fieldName, cidr, err)
		}
	}
	return nil
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
	if c.ProxyAuth.Enabled && len(c.ProxyAuth.TrustedCIDRs) == 0 {
		return errors.New("proxy_auth.trusted_cidrs must be set when proxy auth is enabled")
	}
	if err := validateCIDRList("tailscale.trusted_cidrs", c.Tailscale.TrustedCIDRs); err != nil {
		return err
	}
	if err := validateCIDRList("proxy_auth.trusted_cidrs", c.ProxyAuth.TrustedCIDRs); err != nil {
		return err
	}
	if c.UI.LinksPerPage < 10 {
		return fmt.Errorf("ui.links_per_page must be >= 10, got %d", c.UI.LinksPerPage)
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
