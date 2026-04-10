# Security

This document describes the security model of golink-url-shortener: which
headers are trusted, which are emitted, how sessions and credentials are
handled, and the known limitations that operators should be aware of.

---

## Authentication modes

Four independent auth modes are available. At most one establishes an identity
for any given request; they run in priority order: Tailscale → proxy auth →
OIDC session cookie → anonymous fallback.

### Anonymous (`anonymous.enabled = true`)

All requests are treated as a single shared identity (`anonymous@localhost`).
There is no per-user isolation. **Never enable this on a publicly reachable
server.** It is intended for local development and air-gapped private instances
only.

### Tailscale (`tailscale.enabled = true`)

The server reads `Tailscale-User-Login`, `Tailscale-User-Name`, and
`Tailscale-User-Profile-Pic` headers injected by the Tailscale HTTP proxy
layer. These headers are only accepted from request connections whose TCP
remote address falls inside one of the `trusted_proxy` CIDR ranges
(configuration validation rejects an empty `trusted_proxy` when this mode is
enabled). See [Trusted-proxy CIDR checks](#trusted-proxy-cidr-checks) below.

> **Note:** Tailscale injects these headers only in `tailscale serve` HTTP
> proxy mode. TCPForward mode passes raw bytes through without injecting any
> headers; requests arriving that way carry no Tailscale identity.

### Proxy auth (`proxy_auth.enabled = true`)

Reads forward-auth headers injected by a trusted reverse proxy (nginx, Caddy,
Traefik, Authelia, …). The same CIDR guard as Tailscale applies — headers from
untrusted IPs are silently ignored. Default header names match Authelia's
convention:

| Header | Config key | Purpose |
|---|---|---|
| `Remote-Email` | `proxy_auth.email_header` | Primary user identifier |
| `Remote-User` | `proxy_auth.user_header` | Fallback identifier when email absent |
| `Remote-Name` | `proxy_auth.name_header` | Display name |
| `Remote-Groups` | `proxy_auth.groups_header` | Comma-separated group list |

### OIDC (`oidc.enabled = true`)

Full OAuth 2.0 / OpenID Connect flow. On successful authentication the server
issues a signed JWT stored in a session cookie (see [Session
cookie](#session-cookie-oidc-only) below). Requires `canonical_address` to be
set so the OIDC provider can redirect back to the correct URL.

---

## Trusted-proxy CIDR checks

For both Tailscale and proxy auth, **all header-based identity claims are
validated against the actual TCP remote address of the connection**, not
against `X-Forwarded-For` (which is trivially spoofable from outside the
proxy).

The implementation works in two steps:

1. `PreserveRemoteAddr` middleware saves `r.RemoteAddr` into the request
   context before chi's `RealIP` middleware can overwrite it with the
   `X-Forwarded-For` value.
2. The Tailscale and proxy-auth middlewares call `PeerIP(r)`, which reads
   this saved original address, and check it against the `trusted_proxy`
   CIDR list. Headers from IPs outside that list are silently dropped and the
   request is treated as unauthenticated.

`X-Forwarded-Proto` is subject to the same guard: the scheme override is only
honoured when the TCP peer falls within `trusted_proxy`.

---

## Response security headers

The following headers are added to every response by the `SecurityHeaders`
middleware:

| Header | Value |
|---|---|
| `X-Frame-Options` | `SAMEORIGIN` |
| `X-Content-Type-Options` | `nosniff` |
| `Referrer-Policy` | `strict-origin-when-cross-origin` |
| `Content-Security-Policy` | see below |
| `Strict-Transport-Security` | `max-age=31536000; includeSubDomains` — **HTTPS only** |

### Content-Security-Policy

```
default-src 'self';
script-src  'self';
style-src   'self';
img-src     'self' data:;
connect-src 'self';
font-src    'self';
object-src  'none';
base-uri    'self';
form-action 'self';
frame-ancestors 'self'
```

No `'unsafe-inline'` is required because all JavaScript and CSS assets are
self-hosted (see `web/static/`) and all former inline `<script>` blocks have
been moved to `web/static/app.js`.

### Strict-Transport-Security

HSTS is emitted **only when `canonical_address` uses the `https://` scheme**.
When the canonical address uses `http://` (typical for Tailscale deployments
where TLS is terminated by Tailscale itself) or is not set at all (local
development), the header is omitted entirely. This prevents browsers from
pinning HSTS for hosts that are intentionally HTTP-only.

---

## Session cookie (OIDC only)

After a successful OIDC login the server issues a `golink_session` cookie
containing a signed JWT.

| Property | Value |
|---|---|
| Algorithm | HS256 (HMAC-SHA-256) |
| Secret | `jwt_secret` config value |
| Lifetime | 24 hours (non-sliding) |
| Cookie flags | `HttpOnly`, `Secure`, `SameSite=Lax` |

The JWT payload includes `email`, `name`, `picture`, `groups`, and `is_admin`.
These claims are populated at login time from the OIDC provider's ID token and
are not re-verified on subsequent requests.

The OAuth2 state cookie (`golink_oauth_state`) used during the login flow has a
5-minute `MaxAge`, is `HttpOnly`/`Secure`/`SameSite=Lax`, and is cleared
immediately in the callback handler.

---

## CSRF protection

All state-mutating HTML form submissions use a double-submit cookie pattern:

1. When a page with a form is rendered, the server generates a random 128-bit
   token, sets it as the `golink_csrf` cookie (`HttpOnly`, `Secure`,
   `SameSite=Lax`), and embeds it as a hidden `csrf_token` field in the form.
2. On POST the server checks that the form field and the cookie value match.

The `SameSite=Lax` flag on session and CSRF cookies already blocks most
cross-site request forgery in modern browsers; the explicit token check is a
defence-in-depth measure.

---

## API keys

- Keys are generated as 128-bit random values and shown **once** to the admin
  at creation time. The plaintext is never stored.
- Only the **SHA-256 hash** of the key is persisted in the database.
  SHA-256 is used instead of a password-hashing function (bcrypt/argon2)
  because API keys are high-entropy random secrets, not low-entropy passwords;
  dictionary and rainbow-table attacks are not applicable.
- Keys are accepted via `X-API-Key: <key>` or `Authorization: Bearer <key>`.
- Each key has a **read-only or read-write scope**. Read-only keys cannot
  create, edit, or delete links and cannot manage API keys.
- Key management (create, list, revoke) is restricted to admin users.

---

## Input validation and open-redirect protection

### Link names

Names are restricted to ASCII alphanumeric characters, hyphens, underscores,
and dots (`[a-zA-Z0-9\-_.]`). Names that match a server endpoint path segment
(`new`, `edit`, `details`, `api`, `auth`, `static`, …) are reserved and
rejected.

### Redirect targets

Stored target URLs are validated on write:

- Scheme must be `http` or `https`.
- `javascript:`, `data:`, and `vbscript:` are explicitly rejected.

### No user-supplied redirect targets

Redirect responses always point to the URL that was stored in the database by
an authenticated user — never to a URL supplied by the unauthenticated requester.
Path suffixes (`go/name/extra`) are appended to the stored base URL; they are
not used as standalone redirect targets and cannot escape the stored prefix.

### Post-login redirect (`?rd=`)

The `rd` parameter accepted on `/auth/login` is restricted to relative paths
that start with `/` and do not start with `//`. This prevents redirecting to
external domains (e.g. `//evil.com`) while preserving deep-link navigation.

---

## Admin access

Admin users are identified by either:
- Their email being listed in `admin_emails` in the config, or
- Membership in one of the OIDC groups listed in `admin_groups`.

Admin privileges are checked at request time for Tailscale and proxy-auth
modes (live config lookup). For OIDC, admin status is embedded in the JWT at
login time (see [JWT claims staleness](#jwt-claims-staleness) below).

Admins can: manage API keys, run import/export, and access all links regardless
of ownership.

---

## Known limitations

### No per-session JWT revocation

Sessions are stateless JWTs. The server holds no session store, so individual
sessions cannot be invalidated without affecting all users.

**To invalidate all active sessions** (e.g. after a credential compromise),
rotate `jwt_secret` in the config and restart the server. Every outstanding
session cookie will immediately become invalid because the HMAC signature will
no longer verify.

There is no mechanism to invalidate a single user's session without rotating
the global secret.

### JWT claims staleness

The `is_admin` flag and OIDC group memberships (`groups`) are embedded in the
JWT at login and not re-checked against the OIDC provider on subsequent
requests.

Practical consequences:
- **Promoting a user to admin** in the OIDC provider or by adding them to
  `admin_emails` takes effect only after their next login (up to 24 hours
  delay for active sessions).
- **Revoking admin access** in the OIDC provider does not take effect until the
  user's session expires — up to 24 hours. To force immediate revocation,
  rotate `jwt_secret`.
- A user removed entirely from the OIDC provider (account deleted/disabled)
  retains access until their session expires, unless the secret is rotated.

### No built-in rate limiting

The server performs no request rate limiting, login throttling, or brute-force
protection on API key lookups. For internet-facing deployments, apply rate
limiting at the reverse proxy or infrastructure level.

### Database encryption at rest

The database (SQLite file or PostgreSQL) is not encrypted by the application.
Link targets, user email addresses, and API key hashes are stored in plaintext.
Apply OS-level or volume-level encryption if the database may be accessed by
untrusted parties.

### Tailscale: HTTP proxy mode only

Tailscale user headers are injected only when using `tailscale serve` in **HTTP
proxy mode** (via `Handlers`). Plain **TCPForward** mode passes raw bytes
without injecting any headers; requests arriving via TCP forwarding carry no
Tailscale identity and are treated as unauthenticated.
