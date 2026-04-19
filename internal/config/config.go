// Package config provides configuration loading and validation for golink-url-shortener.
package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/mkende/golink-url-shortener/internal/links"
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
}

// ProxyAuthConfig holds settings for reverse-proxy header-based authentication.
// This is similar to Tailscale auth but reads generic forward-auth headers
// injected by a trusted reverse proxy such as nginx, Caddy, or Traefik.
// The header names default to the de-facto standard used by Authelia.
type ProxyAuthConfig struct {
	// Enabled controls whether proxy header-based auth is active.
	Enabled bool `toml:"enabled"`
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
	// ClientSecret is the OAuth2 client secret. Mutually exclusive with
	// ClientSecretEnvVar; exactly one must be set when OIDC is enabled.
	ClientSecret string `toml:"client_secret"`
	// ClientSecretEnvVar is the name of an environment variable whose value is
	// used as the OAuth2 client secret. Mutually exclusive with ClientSecret.
	ClientSecretEnvVar string `toml:"client_secret_env_var"`
	// Scopes is the list of OAuth2 scopes to request.
	// Defaults to ["openid", "email", "profile"].
	Scopes []string `toml:"scopes"`
	// GroupsClaim is the JWT claim name that contains group memberships.
	// Defaults to "groups".
	GroupsClaim string `toml:"groups_claim"`
	// UsePKCE controls whether PKCE (Proof Key for Code Exchange) is used in the
	// OAuth2 authorization code flow. When enabled, a code_verifier and
	// code_challenge are generated and used during the login flow.
	// Defaults to false.
	UsePKCE bool `toml:"use_pkce"`
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

	// CanonicalAddress is the public base URL for this instance, including
	// scheme. Example: "https://go.example.com" or "http://go".
	// Required when OIDC is enabled (needed to build the callback URL).
	// When set, all non-redirect requests that arrive on a different scheme or
	// host are redirected here with a 301.
	CanonicalAddress string `toml:"canonical_address"`

	// TrustedProxy is the list of IP ranges (CIDR notation) from which
	// proxy-forwarding headers are trusted: X-Forwarded-Proto for scheme
	// detection, Tailscale-User-* for Tailscale auth, and Remote-* for
	// proxy_auth. Required when proxy_auth is enabled.
	TrustedProxy []string `toml:"trusted_proxy"`

	// Title is the human-readable name shown in the UI.
	// Defaults to "GoLink".
	Title string `toml:"title"`

	// FaviconPath is a filesystem path to a custom favicon file.
	// An empty string means the embedded default favicon is used.
	FaviconPath string `toml:"favicon_path"`

	// JWTSecret is the HMAC secret used to sign and verify session JWT cookies.
	// Required when OIDC is enabled. Use a long random string (>= 32 bytes).
	// Mutually exclusive with JWTSecretEnvVar; exactly one must be set when
	// OIDC is enabled.
	JWTSecret string `toml:"jwt_secret"`

	// JWTSecretEnvVar is the name of an environment variable whose value is
	// used as the JWT HMAC secret. Mutually exclusive with JWTSecret.
	JWTSecretEnvVar string `toml:"jwt_secret_env_var"`

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

	// AdminGroups is a list of OIDC group names whose members are treated as
	// admins. Requires OIDC (or proxy_auth with groups) to be enabled and the
	// groups_claim to be correctly configured.
	AdminGroups []string `toml:"admin_groups"`

	// CacheSize is the maximum number of links kept in the in-process LRU
	// redirect cache. Increasing this reduces database reads on the hot path.
	// Defaults to 1000.
	CacheSize int `toml:"cache_size"`

	// CacheTTL is the maximum time a link is kept in the redirect cache before
	// being evicted and re-fetched from the database on next access. Use Go
	// duration syntax, e.g. "5m", "1h", "30s". An empty string or "0" disables
	// time-based expiry (entries are only evicted by LRU pressure). Defaults to
	// "1m".
	CacheTTL string `toml:"cache_ttl"`

	// CacheTTLDuration is CacheTTL parsed into a time.Duration. It is populated
	// by Load and must not be set directly in the config file.
	CacheTTLDuration time.Duration `toml:"-"`

	// MaxAliasesPerLink is the maximum number of alias links that may target
	// any single canonical link.  Defaults to 100.
	MaxAliasesPerLink int `toml:"max_aliases_per_link"`

	// LogLevel controls the minimum severity of log messages emitted by the
	// server. Supported values (from most to least verbose): "debug", "info",
	// "warn", "error". Defaults to "info".
	LogLevel string `toml:"log_level"`

	// AllowAdvancedLinks controls whether advanced (Go template) links may be
	// created or followed. When explicitly set to false, the advanced link
	// option is hidden from creation and edit forms, and following an existing
	// advanced link returns an error page instead of performing the redirect.
	// Default: true (advanced links are permitted).
	AllowAdvancedLinks *bool `toml:"allow_advanced_links"`

	// DomainsForAdvancedLinks restricts advanced links to a set of allowed
	// destination domains. Each entry is an exact hostname ("example.com") or
	// a leading-wildcard hostname ("*.example.com"). The wildcard prefix may
	// contain dots (multi-level subdomains) but must not contain slashes or
	// other special characters. When non-empty, an advanced link whose
	// resolved URL does not match any listed domain is rejected at creation
	// time (best-effort dry-run) and at redirect time.
	// Security recommendation: restrict this to internal, trusted domains when
	// advanced links are enabled in production.
	// Default: [] (no domain restriction).
	DomainsForAdvancedLinks []string `toml:"domains_for_advanced_links"`

	// UI holds settings that control the behaviour of the web UI.
	UI UIConfig `toml:"ui"`
}

// UIConfig holds settings that control the behaviour of the web UI.
type UIConfig struct {
	// LinksPerPage is the number of links shown on each page of the /links and
	// /mylinks lists. Must be >= 10. Defaults to 50.
	LinksPerPage int `toml:"links_per_page"`
}

// CanonicalHost returns the host component of CanonicalAddress, or "" if not set.
func (c *Config) CanonicalHost() string {
	if c.CanonicalAddress == "" {
		return ""
	}
	u, err := url.Parse(c.CanonicalAddress)
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Host
}

// CanonicalScheme returns the scheme component of CanonicalAddress, or "" if not set.
func (c *Config) CanonicalScheme() string {
	if c.CanonicalAddress == "" {
		return ""
	}
	u, err := url.Parse(c.CanonicalAddress)
	if err != nil || u.Scheme == "" {
		return ""
	}
	return u.Scheme
}

// AdvancedLinksAllowed returns true when advanced (Go template) links are
// permitted. Returns true by default when allow_advanced_links is not
// explicitly set in the configuration file.
func (c *Config) AdvancedLinksAllowed() bool {
	return c.AllowAdvancedLinks == nil || *c.AllowAdvancedLinks
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
	if c.CacheTTL == "" {
		c.CacheTTL = "1m"
	}
	if c.MaxAliasesPerLink == 0 {
		c.MaxAliasesPerLink = 100
	}
	if c.LogLevel == "" {
		c.LogLevel = "info"
	}
	if c.UI.LinksPerPage == 0 {
		c.UI.LinksPerPage = 50
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

// resolveSecret resolves a single secret field. It returns the direct value
// when envVarName is empty, reads the named environment variable when it is
// set, and returns an error when both are provided or the variable is unset.
// field and envVarField are used only in error messages.
func resolveSecret(field, value, envVarField, envVarName string) (string, error) {
	if value != "" && envVarName != "" {
		return "", fmt.Errorf("cannot set both %s and %s", field, envVarField)
	}
	if envVarName == "" {
		return value, nil
	}
	resolved, ok := os.LookupEnv(envVarName)
	if !ok {
		return "", fmt.Errorf("%s: environment variable %q is not set", envVarField, envVarName)
	}
	return resolved, nil
}

// resolveSecrets populates JWTSecret and OIDC.ClientSecret from the
// corresponding _env_var fields when those are used instead of inline values.
// It returns an error if both forms are provided for the same secret.
func resolveSecrets(c *Config) error {
	var err error
	c.JWTSecret, err = resolveSecret("jwt_secret", c.JWTSecret, "jwt_secret_env_var", c.JWTSecretEnvVar)
	if err != nil {
		return err
	}
	c.OIDC.ClientSecret, err = resolveSecret("oidc.client_secret", c.OIDC.ClientSecret, "oidc.client_secret_env_var", c.OIDC.ClientSecretEnvVar)
	return err
}

// validate checks that required fields are present and values are within
// acceptable ranges. It returns a descriptive error for the first violation
// found.
func validate(c *Config) error {
	if c.CanonicalAddress != "" {
		u, err := url.Parse(c.CanonicalAddress)
		if err != nil || u.Host == "" || u.Scheme == "" {
			return fmt.Errorf("canonical_address must be a valid URL with scheme and host (got %q)", c.CanonicalAddress)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return fmt.Errorf("canonical_address scheme must be http or https (got %q)", u.Scheme)
		}
	}
	if c.OIDC.Enabled && c.CanonicalAddress == "" {
		return errors.New("canonical_address is required when oidc is enabled")
	}
	if !c.Anonymous.Enabled && !c.Tailscale.Enabled && !c.ProxyAuth.Enabled && !c.OIDC.Enabled {
		return errors.New("at least one authentication provider must be enabled (anonymous, tailscale, proxy_auth, or oidc)")
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
	if c.OIDC.Enabled && c.JWTSecret == "" {
		return errors.New("jwt_secret is required when OIDC is enabled")
	}
	if c.Tailscale.Enabled && len(c.TrustedProxy) == 0 {
		return errors.New("trusted_proxy must be set when tailscale auth is enabled")
	}
	if c.ProxyAuth.Enabled && len(c.TrustedProxy) == 0 {
		return errors.New("trusted_proxy must be set when proxy_auth is enabled")
	}
	if err := validateCIDRList("trusted_proxy", c.TrustedProxy); err != nil {
		return err
	}
	if c.UI.LinksPerPage < 10 {
		return fmt.Errorf("ui.links_per_page must be >= 10, got %d", c.UI.LinksPerPage)
	}
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
		// valid
	default:
		return fmt.Errorf("log_level must be one of \"debug\", \"info\", \"warn\", \"error\", got %q", c.LogLevel)
	}
	if c.CacheTTL != "" && c.CacheTTL != "0" {
		d, err := time.ParseDuration(c.CacheTTL)
		if err != nil {
			return fmt.Errorf("cache_ttl: %w", err)
		}
		if d < 0 {
			return fmt.Errorf("cache_ttl must be non-negative, got %q", c.CacheTTL)
		}
		c.CacheTTLDuration = d
	}
	for _, pattern := range c.DomainsForAdvancedLinks {
		if err := links.ValidateDomainPattern(pattern); err != nil {
			return fmt.Errorf("domains_for_advanced_links: %w", err)
		}
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
	meta, err := toml.Decode(string(data), &cfg)
	if err != nil {
		return nil, fmt.Errorf("parsing config file %q: %w", path, err)
	}
	if unknown := meta.Undecoded(); len(unknown) > 0 {
		return nil, fmt.Errorf("parsing config file %q: unknown configuration key(s): %v", path, unknown)
	}

	applyDefaults(&cfg)

	if err := resolveSecrets(&cfg); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	if err := validate(&cfg); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &cfg, nil
}
