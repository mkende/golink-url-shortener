# Redirect Patterns

This page explains how golink-url-shortener processes redirects for both simple
and advanced links.

---

## Simple links

When you visit `go/name`, you are redirected to the link's target URL.

### Path passthrough

Append a path suffix to a simple link and it is appended to the target:

| You visit | Target | Redirected to |
|-----------|--------|---------------|
| `go/docs` | `https://docs.example.com` | `https://docs.example.com` |
| `go/docs/api` | `https://docs.example.com` | `https://docs.example.com/api` |
| `go/docs/api/v2` | `https://docs.example.com` | `https://docs.example.com/api/v2` |

### Fragment passthrough

A `#fragment` in the request URL is appended to the target:

| You visit | Redirected to |
|-----------|---------------|
| `go/docs#section-2` | `https://docs.example.com#section-2` |

---

## Advanced links

When a link is marked **advanced**, the target field is a
[Go template](https://pkg.go.dev/text/template) evaluated at redirect time.
This enables dynamic redirects based on the path, query string, browser, or
authenticated user.

### Template variables

The following variables are available inside the template:

| Variable | Type | Description |
|----------|------|-------------|
| `.path` | string | Path suffix after the link name, without leading slash (e.g. `PROJ-123` for `go/name/PROJ-123`) |
| `.parts` | []string | Path suffix split by `/` (e.g. `["foo", "bar"]` for `go/name/foo/bar`) |
| `.args` | string | Raw query string (everything after `?`) |
| `.ua` | string | `User-Agent` header value |
| `.email` | string | Authenticated user's email address; empty for anonymous users |

### Custom template functions

Three regexp helper functions are provided in addition to the standard Go
template functions:

| Function | Signature | Description |
|----------|-----------|-------------|
| `match` | `match "pattern" s` | Returns `true` if `s` contains a match for the regexp `pattern` (partial match unless the pattern is anchored with `^`/`$`). |
| `extract` | `extract "pattern" s` | Returns the text of the first capturing group in `pattern` when matched against `s`, or an empty string if there is no match. |
| `replace` | `replace "pattern" "repl" s` | Returns `s` with all matches of `pattern` replaced by `repl`. Supports `$1`, `$2`, etc. backreferences. |

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

## Link name rules

- ASCII alphanumeric characters plus `-`, `_`, and `.` only.
- No `/` or `#` in names.
- Case-insensitive: `go/Docs` and `go/docs` resolve to the same link.
- Reserved words (server endpoints such as `/new`, `/edit`, `/create`, etc.)
  may not be used as link names.
