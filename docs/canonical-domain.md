# Canonical Domain and HTTPS Enforcement

This document explains how golink-url-shortener enforces the use of a single
canonical domain and HTTPS scheme, and under which conditions a user is
redirected.

## Configuration

Two config keys control this behaviour (see `config.template.toml` for full
details):

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `canonical_domain` | string | — | **Required.** The public hostname of the service, without a scheme (e.g. `go.example.com`). Used to enforce HTTPS redirects, build OIDC callback URLs, and construct absolute login URLs. |
| `allow_http` | bool | `false` | When `true`, disables automatic HTTPS redirection. Useful for local development or deployments behind a TLS-terminating proxy that does not forward `X-Forwarded-Proto`. |

## When is the redirect applied?

The `DomainRedirect` middleware issues a **301 Moved Permanently** redirect to
`https://<canonical_domain><same-path>?<same-query>` whenever all of the
following conditions are true:

1. `canonical_domain` is set in config.
2. `allow_http` is `false`.
3. The request is **not** already on HTTPS pointing at the canonical domain —
   that is, either the scheme is not HTTPS, or the `Host` header differs from
   `canonical_domain`.

This applies regardless of auth source — Tailscale and reverse-proxy auth users
are subject to the same redirect as everyone else for UI pages.

HTTPS is detected via `r.TLS` (direct TLS) or the `X-Forwarded-Proto: https`
header set by a terminating proxy.

## Which routes are subject to the redirect?

The middleware is applied to **all routes except**:

| Route | Reason for exemption |
|-------|----------------------|
| `/{name}` and `/{name}/*` (link redirects) | Redirect requests are served directly so short links work from any network. |
| `/favicon.ico` | Served early; no canonical-domain requirement. |
| `/healthz` | Health checks must be reachable on any address. |
| `/auth/login`, `/auth/callback`, `/auth/logout` | These must be reachable on whatever URL is registered with the OIDC provider. |

All UI pages (`/`, `/new`, `/edit`, `/browse`, `/admin`, …) and all API
endpoints (`/api/…`) **are** subject to the redirect.

## Exempt cases

### `allow_http = true`

Setting `allow_http = true` disables the entire redirect check. The service
will serve all routes on whatever scheme and host the request arrives on.

## OIDC login and the canonical domain

Link redirects are themselves exempt from the domain middleware, but if a link
(or the global `require_auth_for_redirects` setting) requires authentication,
the service needs to redirect the browser to the OIDC login page. Because the
session cookie is scoped to `canonical_domain`, the login URL is always built
as an **absolute URL on the canonical domain**:

```
https://<canonical_domain>/auth/login?rd=<original-request-uri>
```

This ensures the OIDC flow completes on the correct domain so the resulting
session cookie is valid for subsequent requests.

This absolute redirect is only issued when both `oidc.enabled = true` and
`canonical_domain` is set. If OIDC is not enabled, a relative `/auth/login?rd=…`
URL is used instead.

## Summary flowchart

```
Incoming request
│
├─ Is it a link redirect (/{name})?    ──YES──► Serve redirect directly (no domain check)
│
├─ Is canonical_domain empty?          ──YES──► Serve request as-is
├─ Is allow_http = true?               ──YES──► Serve request as-is
│
├─ Is request on HTTPS + correct host? ──YES──► Serve request normally
│
└─ Otherwise ──► 301 to https://<canonical_domain><path>?<query>
```
