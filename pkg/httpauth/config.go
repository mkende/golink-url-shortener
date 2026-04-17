package httpauth

import (
	"fmt"
	"os"
)

// AuthConfig is the serialisable authentication configuration. It is designed
// to map 1:1 to a [auth] section in a TOML config file so that it can be
// embedded directly in an application's Config struct.
//
// Runtime-only values (canonical address, trusted proxy CIDRs, JWT secret)
// are NOT part of AuthConfig; pass them as options to [New] instead.
type AuthConfig struct {
	// AdminEmails lists the email addresses that are granted admin privileges.
	AdminEmails []string `toml:"admin_emails"`
	// AdminGroups lists the group names whose members are granted admin
	// privileges. Requires a provider that supplies group information (OIDC
	// with a groups claim, or proxy auth with a groups header).
	AdminGroups []string `toml:"admin_groups"`

	// OIDC holds OpenID Connect authentication settings.
	OIDC OIDCConfig `toml:"oidc"`
	// Tailscale holds Tailscale header-based authentication settings.
	Tailscale TailscaleConfig `toml:"tailscale"`
	// ProxyAuth holds reverse-proxy forward-auth header settings.
	ProxyAuth ProxyAuthConfig `toml:"proxy_auth"`
	// Anonymous holds anonymous shared-identity settings.
	Anonymous AnonymousConfig `toml:"anonymous"`
}

// OIDCConfig holds settings for OpenID Connect authentication.
type OIDCConfig struct {
	// Enabled controls whether OIDC authentication is active.
	Enabled bool `toml:"enabled"`
	// Issuer is the OIDC provider issuer URL (e.g. "https://accounts.google.com").
	Issuer string `toml:"issuer"`
	// ClientID is the OAuth2 client identifier.
	ClientID string `toml:"client_id"`
	// ClientSecret is the OAuth2 client secret. Mutually exclusive with
	// ClientSecretEnvVar; exactly one must be set when OIDC is enabled.
	ClientSecret string `toml:"client_secret"`
	// ClientSecretEnvVar is the name of an environment variable whose value is
	// used as the client secret. Mutually exclusive with ClientSecret.
	ClientSecretEnvVar string `toml:"client_secret_env_var"`
	// Scopes is the list of OAuth2 scopes to request.
	// Defaults to ["openid", "email", "profile"].
	Scopes []string `toml:"scopes"`
	// GroupsClaim is the JWT claim name that contains group memberships.
	// Defaults to "groups".
	GroupsClaim string `toml:"groups_claim"`
}

// TailscaleConfig holds settings for Tailscale header-based authentication.
// This provider reads the Tailscale-User-Login, Tailscale-User-Name, and
// Tailscale-User-Profile-Pic headers injected by `tailscale serve` in HTTP
// proxy mode. Plain TCP forwarding does not inject these headers.
type TailscaleConfig struct {
	// Enabled controls whether Tailscale header-based auth is active.
	Enabled bool `toml:"enabled"`
}

// ProxyAuthConfig holds settings for reverse-proxy forward-auth header
// authentication. The header names default to the de-facto standard used by
// Authelia (Remote-User, Remote-Email, Remote-Name, Remote-Groups).
type ProxyAuthConfig struct {
	// Enabled controls whether proxy header-based auth is active.
	Enabled bool `toml:"enabled"`
	// UserHeader is the header containing the authenticated user's login name.
	// Used as the primary identifier when EmailHeader is absent. Defaults to
	// "Remote-User".
	UserHeader string `toml:"user_header"`
	// EmailHeader is the header containing the user's email address. When
	// present it takes precedence over UserHeader as the primary identifier.
	// Defaults to "Remote-Email".
	EmailHeader string `toml:"email_header"`
	// NameHeader is the header containing the user's display name.
	// Defaults to "Remote-Name".
	NameHeader string `toml:"name_header"`
	// GroupsHeader is the header containing a comma-separated list of group
	// memberships. Defaults to "Remote-Groups".
	GroupsHeader string `toml:"groups_header"`
}

// AnonymousConfig holds settings for anonymous (user-less) authentication.
// In this mode every request is treated as a single shared anonymous user.
// Intended for local development, testing, or isolated private instances.
//
// WARNING: Do not enable on a publicly reachable server.
type AnonymousConfig struct {
	// Enabled controls whether anonymous auth is active.
	Enabled bool `toml:"enabled"`
	// IsAdmin grants the anonymous user full admin privileges when true.
	IsAdmin bool `toml:"is_admin"`
}

// ApplyAuthDefaults fills in zero-value AuthConfig fields with their documented
// defaults. It is called automatically by [New]; applications that load config
// before constructing an [AuthManager] (e.g. in a config.Load function) may
// call it to have the populated defaults visible in the loaded config.
func ApplyAuthDefaults(c *AuthConfig) { applyAuthDefaults(c) }

// applyAuthDefaults fills in zero-value AuthConfig fields with their defaults.
func applyAuthDefaults(c *AuthConfig) {
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
}

// ResolveAuthSecrets resolves any env-var-based secrets in cfg in place. It
// should be called by the application's config loader so that misconfigured
// environment variables are caught at startup rather than at first use.
//
// [New] calls this automatically, so calling it again later is a no-op.
func ResolveAuthSecrets(cfg *AuthConfig) error {
	return resolveOIDCClientSecret(&cfg.OIDC)
}

// resolveOIDCClientSecret resolves OIDC.ClientSecret from the corresponding
// env-var field when that is used instead of an inline value. Returns an error
// when both are provided or the env var is unset.
func resolveOIDCClientSecret(c *OIDCConfig) error {
	if c.ClientSecret != "" && c.ClientSecretEnvVar != "" {
		return fmt.Errorf("cannot set both oidc.client_secret and oidc.client_secret_env_var")
	}
	if c.ClientSecretEnvVar == "" {
		return nil
	}
	val, ok := os.LookupEnv(c.ClientSecretEnvVar)
	if !ok {
		return fmt.Errorf("oidc.client_secret_env_var: environment variable %q is not set", c.ClientSecretEnvVar)
	}
	c.ClientSecret = val
	return nil
}
