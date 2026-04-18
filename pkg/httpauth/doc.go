// Package httpauth provides pluggable HTTP authentication middleware and
// helpers for Go HTTP servers.
//
// # Quick start
//
// Create an [AuthManager] once at startup, then wire it into your router:
//
//	mgr, err := httpauth.New(ctx, authCfg,
//	    httpauth.WithCanonicalAddress("https://go.example.com"),
//	    httpauth.WithJWTSecret(secret),
//	    httpauth.WithTrustedProxy([]string{"10.0.0.0/8"}),
//	    httpauth.WithLogger(logger),
//	)
//
//	r.Use(mgr.Middleware())   // global: logging, security headers, auth providers
//	mgr.Mount(r)              // registers /auth/login, /auth/callback, /auth/logout
//
//	r.Group(func(r chi.Router) {
//	    r.Use(mgr.DomainRedirect())
//	    r.Use(mgr.RequireAuth(deniedHandler))
//	    // ... protected routes ...
//	})
//
// # Authentication providers
//
// Four providers are supported; they run in this order and each one skips if
// a prior provider already set an identity:
//
//   - Tailscale header auth ([TailscaleConfig])
//   - Reverse-proxy forward-auth headers ([ProxyAuthConfig])
//   - OpenID Connect / OAuth2 ([OIDCConfig])
//   - Anonymous shared identity ([AnonymousConfig])
//
// Enable one or more in [AuthConfig]. At least one must be enabled.
//
// # Configuration
//
// [AuthConfig] is designed to be loaded directly from a TOML (or any other)
// configuration file — it contains only serialisable values. Runtime-only
// values (canonical address, trusted proxy CIDRs, JWT secret) are passed as
// functional options to [New].
//
// # Identity
//
// Every authenticated request carries an [Identity] in its context. Retrieve
// it with [IdentityFromContext]. Enforcement middleware ([AuthManager.RequireAuth],
// [AuthManager.RequireAdmin], [AuthManager.RequireWriteScope]) use the same
// context value and do not need to be told which provider was used.
package httpauth
