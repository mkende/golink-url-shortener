# Canonical Domain, HTTPS Enforcement and auth support

## Configuration options

- `canonical_address`: string, empty by default, otherwise a scheme + domain.
   Example: http://go, https://go.example.com
- `trusted_proxy`: list of strings, empty by default (replace the current
   trusted_cidrs options of the auth providers, that should be replaced by that
   single option).
-  `require_auth_for_redirects`: boolean, false by default.

None of these are required, except that `canonical_address` is required if and
only if the `oidc` provider is specified (see below), as we need it to build the
correct callback URL.

The current `allow_logged_out_ui_access` option should be removed, we will
consider that it’s always false. Same for `allow_http` that no longer exist.
Instead we just rely on the `canonical_address` and its included scheme.

## Request state

On incoming request, we extract from the request data the following information:

- `is_https`
- `requested_domain`

If and only if `trusted_proxy` is non-empty and the peer IP matches one of the
specified range. Then we trust the `X-Forwarded-Proto` headers and such to
override that information.

## Authentication provider

We have the following 4 authentication providers that can be individually
enabled.

- `tailscale`: trust the `Tailscale-User-Xxx` headers. You need to pass the
  `localhost` IP in the `trusted_proxy` configuration.
- `oidc`: connects to an OIDC provider.
- `proxy_auth`: trust specified headers, typically `Remote-Xxx`, from host that
  are specified with the `trusted_proxy` config.
- `anonymous`: authenticate all incoming traffic as a single anonymous user.

At least one authentication provider is required (the server fails to start if
none are specified).

## Request flow

When a request arrive to the server, the following flow is applied:

1. Try to authenticate the user with our active plugins.
2. If the current request is for an existing link which does not require auth
   and the global `require_auth_for_redirects` is not set, then redirect the
   user to the link target.
3. Otherwise, if `canonical_address` is set and the current request does not
   match it (protocol + domain as read in the request state described above),
   then we redirect the user to the canonical_address, keeping the same path.
4. At this point, we are either authenticated or not (as we have the right
   cookie for our domain).
5. If we are not authenticated and the request is for a link that requires auth,
   or it’s for a link redirect and the global `require_auth_for_redirects` is
   set, or it’s for any other UI page, then we show a (nice, following our site
   template) "unauthorized" page. If the OIDC provider is set, then we redirect
   the user to the login flow instead.
6. At this point, we only have authenticated users (other users have either been
   redirected or been shown a permission denied page).
7. If the request is for an admin page and the user is not an admin, we show
   another unauthorized page.
8. Otherwise we show the page requested by the user, or redirect it to the
   link destination.

Let me know if I forgot an option in this flow.

This document explains how golink-url-shortener enforces the use of a single
canonical domain and HTTPS scheme, and under which conditions a user is
redirected.

Note: a small subset of route are exempted from that logic and always available.
These are the `/healthz` and `/favicon.ico` one, as well as all those required
for OIDC to function.
