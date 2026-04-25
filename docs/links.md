# Redirect Patterns

This page explains how golink-url-shortener processes redirects for both simple
and advanced links.

---

## Simple links

When you visit `go/name`, you are redirected to the link's target URL.

### Path passthrough

Append a path suffix to a simple link and it is appended to the target. For a
link `docs` with target `https://docs.example.com`:

- `go/docs` → `https://docs.example.com`
- `go/docs/api` → `https://docs.example.com/api`
- `go/docs/api/v2` → `https://docs.example.com/api/v2`

### Fragment passthrough

A `#fragment` in the request URL is appended to the target:

- `go/docs#section-2` → `https://docs.example.com#section-2`

---

## Advanced links

When a link is marked **advanced**, the target field is a
[Go template](https://pkg.go.dev/text/template) evaluated at redirect time.
This enables dynamic redirects based on the path, query string, browser, or
authenticated user.

### Template variables

The following variables are available inside the template:

- **`.path`** (string) — Path suffix after the link name, without leading slash
  (e.g. `PROJ-123` for `go/name/PROJ-123`).
- **`.parts`** ([]string) — Path suffix split by `/`
  (e.g. `["foo", "bar"]` for `go/name/foo/bar`).
- **`.args`** ([]string) — Query string split on `&`
  (e.g. `["q=hello", "page=1"]` for `go/name?q=hello&page=1`).
- **`.ua`** (string) — `User-Agent` header value.
- **`.email`** (string) — Authenticated user's email address; empty for
  anonymous users.
- **`.alias`** (string) — The short link name used in the request. When the
  user visits via an alias this is the alias name; when visiting directly it is
  the canonical link name. Useful to build templates that behave differently
  depending on which name was used.

### Custom template functions

Three regexp helper functions are provided in addition to the standard Go
template functions:

- **`match "pattern" s`** — Returns `true` if `s` contains a match for the
  regexp `pattern` (partial match unless the pattern is anchored with `^`/`$`).
- **`extract "pattern" s`** — Returns the text of the first capturing group in
  `pattern` when matched against `s`, or an empty string if there is no match.
- **`replace "pattern" "repl" s`** — Returns `s` with all matches of `pattern`
  replaced by `repl`. Supports `$1`, `$2`, etc. backreferences.

---

## Common examples

### Jira ticket lookup

Link name: `jira`
Target:

```
https://jira.example.com/browse/{{index .parts 0}}
```

Usage: `go/jira/PROJ-123` → `https://jira.example.com/browse/PROJ-123`

---

### GitHub pull request

Link name: `pr`
Target:

```
https://github.com/myorg/myrepo/pull/{{index .parts 0}}
```

Usage: `go/pr/456` → `https://github.com/myorg/myrepo/pull/456`

---

### GitHub repository search

Link name: `gh`
Target:

```
https://github.com/{{index .parts 0}}/{{index .parts 1}}
```

Usage: `go/gh/myorg/myrepo` → `https://github.com/myorg/myrepo`

---

### Confluence page search

Link name: `wiki`
Target:

```
https://wiki.example.com/dosearchsite.action?queryString={{.path}}
```

Usage: `go/wiki/onboarding` → search Confluence for "onboarding"

---

### Conditional redirect by user

Link name: `dashboard`
Target (redirects each user to their own dashboard):

```
https://app.example.com/users/{{.email}}/dashboard
```

---

### Conditional redirect by browser

Link name: `download`
Target (detects mobile):

```
{{if match "Mobile|Android|iPhone" .ua}}https://m.example.com/download{{else}}https://example.com/download{{end}}
```

---

### Extract a ticket number from a mixed path

Link name: `ticket`
Target:

```
https://tracker.example.com/issues/{{extract "([A-Z]+-[0-9]+)" .path}}
```

Usage: `go/ticket/see-PROJ-99-for-details` → `https://tracker.example.com/issues/PROJ-99`

---

## Advanced link security

Advanced links are powerful but carry a higher security risk than simple links:
any user who can create an advanced link can redirect other users to an
arbitrary URL through template logic. Administrators can restrict this risk in
two complementary ways via `golink.conf`:

**Disable entirely** (`allow_advanced_links = false`):
The advanced link type is hidden from creation and edit forms. Existing
advanced links return an error page when followed, prompting their owners to
convert them to simple links.

**Restrict to allowed domains** (`domains_for_advanced_links`):
Advanced links may only redirect to the listed hostnames. Entries may be exact
(`example.com`) or use a leading wildcard (`*.example.com`), where the
wildcard matches any non-empty subdomain (including multi-level ones such as
`a.b.example.com`). Creating a link whose dry-run resolved URL falls outside
the list is rejected; following such a link at runtime also shows an error page.

**Recommendation**: if advanced links are enabled in a production deployment,
set `domains_for_advanced_links` to a list of internal, trusted hostnames so
that advanced links cannot be used to redirect users to arbitrary external
sites.

---

## Link name rules

- ASCII alphanumeric characters plus `-`, `_`, and `.` only.
- No `/` or `#` in names.
- Case-insensitive: `go/Docs` and `go/docs` resolve to the same link.
- Reserved words (server endpoints such as `/new`, `/edit`, `/create`, etc.)
  may not be used as link names.

---

## Link Sharing

By default, a link is only editable by its owner. You can share a link with
other users, granting them permission to view and edit it.

On a link's detail page, the owner (and any user the link is already shared
with) can add additional users or groups to the share list. Enter the email
address of the user you want to share with, or select from the auto-complete
suggestions.

If your instance uses OIDC authentication with group support, you can also share
with entire groups by entering the group name.

On the same detail page, you can see the full list of users and groups the link
is shared with. Remove any entry to revoke access.

- The link owner can always edit the link and manage shares.
- Any user in the share list can edit the link and add/remove other shares.
- Admins can access and edit any link, including managing its shares.
