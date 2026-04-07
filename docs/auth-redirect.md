# Authentication and Redirect Flow

This document describes how golink-url-shortener handles authentication,
canonical domain enforcement, and HTTPS redirection.

## Configuration

- **`canonical_address`** — Canonical scheme + host, e.g. `"https://go.example.com"`
  or `"http://go"`. Required when `oidc` is enabled. When set, non-public
  requests on a different scheme or host are redirected here with a 301.
- **`trusted_proxy`** — CIDR ranges of trusted reverse proxies. When the peer IP
  matches, `X-Forwarded-Proto` is trusted for scheme detection and
  provider-injected headers (`Tailscale-User-*`, `Remote-*`) are accepted.
  Required when `proxy_auth` is enabled. Default: `[]`.
- **`require_auth_for_redirects`** — When `true`, unauthenticated users cannot
  follow any redirect. Default: `false`.

## Authentication providers

At least one provider must be enabled; the server refuses to start otherwise.

- **`tailscale`** — Trusts `Tailscale-User-*` headers. Requires `trusted_proxy`
  to include the Tailscale CGNAT range or `127.0.0.0/8`.
- **`proxy_auth`** — Trusts `Remote-*` headers (Authelia convention) from IPs in
  `trusted_proxy`. Requires `trusted_proxy` to be set.
- **`oidc`** — Full OIDC login flow. Requires `canonical_address`.
- **`anonymous`** — Treats every request as a single shared anonymous user. For
  local/private instances only.

## Request state extraction

On each incoming request the server determines:

- **`is_https`** — whether the request arrived over HTTPS.
- **`requested_domain`** — the host the client used.

`X-Forwarded-Proto` is only trusted to override `is_https` when `trusted_proxy`
is non-empty and the peer IP falls within one of the configured ranges.

## Request flow

1. **Authenticate** — run all enabled providers; attach identity to the request
   context if any succeeds.
2. **Public link fast-path** — if the request targets an existing link that does
   not require authentication and `require_auth_for_redirects` is `false`,
   redirect immediately to the link target (skip steps 3–6).
3. **Canonical redirect** — if `canonical_address` is set and the request does
   not match its scheme and host, redirect to the canonical URL (301), preserving
   the path.
4. **Auth check** — if the user is not authenticated:
   - If OIDC is enabled, redirect to the OIDC login flow.
   - Otherwise, render a styled "Unauthorized" page (no login button).
5. **Admin check** — if the request targets an admin page and the authenticated
   user is not an admin, render a styled "Forbidden" page.
6. **Serve** — serve the requested UI page or perform the link redirect.

> **Exemptions:** `/healthz`, `/favicon.ico`, and all OIDC endpoints
> (`/auth/login`, `/auth/callback`, `/auth/logout`) bypass this flow entirely.
