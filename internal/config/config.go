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
	"github.com/mkende/golink-url-shortener/pkg/httpauth"
)

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
	// proxy_auth. Required when proxy_auth or tailscale auth is enabled.
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

	// Auth holds all authentication provider configuration.
	Auth httpauth.AuthConfig `toml:"auth"`

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

	// CacheSize is the maximum number of links kept in the in-process LRU
	// redirect cache. Defaults to 1000.
	CacheSize int `toml:"cache_size"`

	// CacheTTL is the maximum time a link is kept in the redirect cache before
	// being evicted and re-fetched from the database on next access. Use Go
	// duration syntax, e.g. "5m", "1h", "30s". Defaults to "1m".
	CacheTTL string `toml:"cache_ttl"`

	// CacheTTLDuration is CacheTTL parsed into a time.Duration. It is populated
	// by Load and must not be set directly in the config file.
	CacheTTLDuration time.Duration `toml:"-"`

	// MaxAliasesPerLink is the maximum number of alias links that may target
	// any single canonical link. Defaults to 100.
	MaxAliasesPerLink int `toml:"max_aliases_per_link"`

	// LogLevel controls the minimum severity of log messages emitted by the
	// server. Supported values: "debug", "info", "warn", "error". Defaults to
	// "info".
	LogLevel string `toml:"log_level"`

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

// applyDefaults fills in zero-value fields with their documented defaults,
// including auth-provider defaults via httpauth.ApplyAuthDefaults.
func applyDefaults(c *Config) {
	httpauth.ApplyAuthDefaults(&c.Auth)
	if c.ListenAddr == "" {
		c.ListenAddr = "0.0.0.0:8080"
	}
	if c.Title == "" {
		c.Title = "GoLink"
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

// validateCIDRList returns an error if any entry in cidrs is not a valid CIDR.
func validateCIDRList(fieldName string, cidrs []string) error {
	for _, cidr := range cidrs {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return fmt.Errorf("%s: %q is not a valid CIDR: %w", fieldName, cidr, err)
		}
	}
	return nil
}

// resolveSecret resolves a single secret field from a direct value or an env var.
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

// resolveSecrets populates JWTSecret from JWTSecretEnvVar when the env-var
// form is used, and delegates OIDC secret resolution to httpauth.
func resolveSecrets(c *Config) error {
	var err error
	c.JWTSecret, err = resolveSecret("jwt_secret", c.JWTSecret, "jwt_secret_env_var", c.JWTSecretEnvVar)
	if err != nil {
		return err
	}
	return httpauth.ResolveAuthSecrets(&c.Auth)
}

// validate checks that required fields are present and values are within
// acceptable ranges. Note: auth-provider-level validation (at least one
// provider, OIDC issuer, etc.) is performed by the AuthManager at construction
// time; this function validates only application-level fields.
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
	if !c.Auth.OIDC.Enabled && !c.Auth.Tailscale.Enabled && !c.Auth.ProxyAuth.Enabled && !c.Auth.Anonymous.Enabled {
		return errors.New("at least one authentication provider must be enabled (auth.oidc, auth.tailscale, auth.proxy_auth, or auth.anonymous)")
	}
	if c.Auth.OIDC.Enabled && c.CanonicalAddress == "" {
		return errors.New("canonical_address is required when auth.oidc is enabled")
	}
	if c.Auth.OIDC.Enabled && c.JWTSecret == "" {
		return errors.New("jwt_secret is required when auth.oidc is enabled")
	}
	if c.Auth.Tailscale.Enabled && len(c.TrustedProxy) == 0 {
		return errors.New("trusted_proxy must be set when auth.tailscale is enabled")
	}
	if c.Auth.ProxyAuth.Enabled && len(c.TrustedProxy) == 0 {
		return errors.New("trusted_proxy must be set when auth.proxy_auth is enabled")
	}
	if err := validateCIDRList("trusted_proxy", c.TrustedProxy); err != nil {
		return err
	}
	if c.QuickLinkLength < 4 {
		return fmt.Errorf("quick_link_length must be >= 4, got %d", c.QuickLinkLength)
	}
	switch c.DB.Driver {
	case "sqlite", "postgres":
	default:
		return fmt.Errorf("db.driver must be \"sqlite\" or \"postgres\", got %q", c.DB.Driver)
	}
	if c.UI.LinksPerPage < 10 {
		return fmt.Errorf("ui.links_per_page must be >= 10, got %d", c.UI.LinksPerPage)
	}
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
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
