# Agent Instructions — golink-redirector

This document contains the standing instructions for any AI agent working on
this project. Read it in full before making any changes. The original
[design-doc.md](design-doc.md) can be consulted for additional context and
intent behind any requirement.

---

## Project summary

`golink-redirector` is a production-grade Go URL shortener (go-link style).
Users browse or are redirected from a short name like `go/docs` to a full URL.
The service features authentication, link sharing, advanced template-based
redirects, a REST API, import/export, and full deployment documentation.

---

## Architecture constraints

- **Language**: Go. No polyglot backend.
- **Database**: SQLite by default; optional PostgreSQL. Never load the entire
  links or users table into memory.
- **Database library**: `database/sql` + `sqlc` (type-safe generated queries).
- **Migrations**: `golang-migrate/migrate` with embedded versioned `.sql` files.
- **HTTP router**: `chi` (lightweight, idiomatic, `net/http`-compatible).
- **Frontend**: server-side `html/template` + HTMX + Bulma CSS (no build step).
- **Sessions**: stateless JWT via `golang-jwt/jwt` v5; stored in a `Secure`,
  `HttpOnly`, `SameSite=Lax` cookie.
- **OIDC**: `coreos/go-oidc` v3 + `golang.org/x/oauth2`.
- **Configuration**: single `simple.conf` file in TOML format. A fully
  documented template file must exist at `config.template.toml`.
- **Auth**: two independent optional modes — Tailscale header-based and OIDC.
  Unauthenticated users may always follow redirects unless the config forbids
  it.
- **Scalability target**: 100 k links, thousands of req/s. No O(n²) algorithms.
  Link hit-count and last-access updates must not cause write contention
  (use async/buffered writes or equivalent).
- **License**: MIT (`LICENSE.txt`). No per-file license headers.

---

## Coding style

- High-quality, idiomatic Go: strong types, proper error handling, no catch-all
  errors, no dummy/placeholder return values left in committed code.
- Follow standard Go project layout (`cmd/`, `internal/`, `docs/`).
- Tests: good coverage of non-trivial logic; no thousand-line test files.
  Table-driven tests preferred.
- No `fmt.Println` in production paths — use structured logging.
- All exported symbols must have doc comments.

---

## Key behavioural rules

### Link names
- ASCII alphanumeric + `-`, `_`, `.` only. No `/` or `#`.
- Case-insensitive: stored as entered, indexed/sorted on lower-case version.
- Any word matching a server endpoint (`/create`, `/edit`, `/new`, …) is
  forbidden as a link name.

### Redirect behaviour (simple links)
- `go/name/extra` → `<target>/extra`
- `go/name#frag` → `<target>#frag`

### Redirect behaviour (advanced links)
- Target is a Go template; extra Go template functions available:
  - `match(pattern, s)` — partial regexp match unless anchored
  - `extract(pattern, s)` — return first submatch
  - `replace(pattern, repl, s)` — regexp replace
- Template variables: `path`, `parts`, `args`, `ua`, `email`.

### Domain redirect rule
For every non-redirect request (UI pages, API, etc.) the server checks:
1. Is the request using the configured canonical HTTPS domain?
2. If not → 301 to the canonical HTTPS URL (same path).
Exception: redirect requests are served directly without this check.
For OIDC auth on links that require authentication: redirect to canonical
domain first so the auth cookie is present.
Tailscale auth: if the Tailscale header is present, skip the redirect check.

### Quick link
Random name of configurable length (default 6), lowercase letters + digits.

### Sharing
- Owner or shared user can edit a link.
- Share by email (or group if OIDC groups are available).
- Config: `default_domain` appended to bare email addresses; `required_domain`
  to restrict to a single domain.
- Auto-complete from known users/groups where possible.

### Admin
- Admin users listed in config or members of a configured OIDC admin group.
- Admin can create/revoke API keys (with descriptive names) on `/apikeys`.
- Import/export restricted to admins.

### API
- REST API on the same paths as the HTML UI; content negotiated by
  `Accept: application/json` or `Content-Type: application/json`.
- Authenticated by API key (Bearer token or `X-API-Key` header).
- Endpoints: create, edit (field-mask), delete, resolve, import, export.

---

## When to ask before acting

**Always ask** the owner before making a choice in any of these areas:
- Any change that affects the public API shape or config file format
- Any new external dependency not already listed in Architecture constraints

For everything else you may proceed, but document your choice briefly in the
commit message.

---

## Security requirements

- Treat this as production-grade software.
- Validate and sanitise all user input (link names, redirect targets, emails).
- Protect against open-redirect abuse (only redirect to explicitly stored URLs;
  do not reflect user-supplied paths as redirect targets).
- Use `http.SetCookie` with `Secure`, `HttpOnly`, `SameSite=Lax` for session
  cookies.
- API keys must be stored hashed; only shown to the admin once on creation.
- Perform a security review pass before closing each major phase.

---

## CI / CD

- GitHub Actions workflow: run all unit tests on every PR targeting `main`.
- Second workflow: build Docker image on push to `main`; publish to `ghcr.io`
  when a `vX.Y.Z` tag is pushed. Cache the build layer between the two jobs.
- CI workflows are added only at the **end** of development (see
  `IMPLEMENTATION_PLAN.md`).

---

## Documentation to maintain

| File | Purpose |
|------|---------|
| `README.md` | Overview + quick-start |
| `docs/deployment.md` | Self-host, Docker, Compose, Kubernetes |
| `docs/configuration.md` | Config reference (may be auto-generated from template) |
| `docs/api.md` | REST API reference |
| `docs/links.md` | Redirect pattern help (same content as `/help` page) |
| `config.template.toml` | Fully documented config template |

---

## Implementation progress

Follow [IMPLEMENTATION_PLAN.md](IMPLEMENTATION_PLAN.md) in phase order.
Mark each checkbox as done when the phase is committed.

---

## Branch & commit hygiene

- Commit messages: imperative mood, ≤ 72 chars subject, blank line, then body
  explaining *why* if non-obvious.
- Each implementation phase ends with the phase number and name in the commit
  subject, e.g. `phase 3: database layer`.
- Never force-push to `main`.
