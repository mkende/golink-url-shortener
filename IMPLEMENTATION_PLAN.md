# Implementation Plan — golink-redirector

Each phase below is a self-contained unit of work that ends with a commit and,
where applicable, passing tests. Complete phases in order; do not skip ahead.
The original [design-doc.md](design-doc.md) can be consulted for additional
context and intent behind any requirement.

---

## Phase 0 — Repository bootstrap

- [x] Add `LICENSE.txt` (MIT).
- [x] Initialise Go module (`go mod init github.com/mkende/golink-redirector`).
- [x] Create top-level directory skeleton:
  ```
  cmd/golink/          main entry-point
  internal/config/     config loading
  internal/db/         database layer
  internal/auth/       authentication
  internal/links/      link business logic
  internal/templates/  HTML + text templates
  internal/api/        HTTP handlers
  internal/redirect/   redirect engine
  docs/
  web/static/          CSS, JS, favicon
  web/templates/       HTML templates
  ```
- [x] Add `.gitignore` (Go binaries, `*.db`, `.env`, editor files).
- [x] Add a minimal `README.md` stub (to be completed in phase 10).
- [x] **Commit**: `phase 0: repository bootstrap`

---

## Phase 1 — Configuration

- [x] Define `Config` struct covering all options:
  - Server: `listen_addr`, `canonical_domain`, `title`, `favicon_path`
  - Auth: `tailscale.enabled`, `oidc.*`, `require_auth_for_redirects`
  - Database: `db.driver` (`sqlite`/`postgres`), `db.dsn`
  - Links: `quick_link_length` (default 6), `default_domain`,
    `required_domain`
  - Admin: `admin_emails`, `admin_group`
- [x] Implement TOML loader with validation and helpful error messages.
- [x] Write `config.template.toml` documenting every option with its default.
- [x] Unit-test config loading (valid, missing required fields, bad types).
- [x] **Commit**: `phase 1: configuration`

---

## Phase 2 — Database layer

- [x] Define schema for:
  - `links` (id, name, name_lower, target, owner_email, is_advanced,
    require_auth, created_at, last_used_at, use_count)
  - `link_shares` (link_id, shared_with_email)
  - `users` (email, display_name, avatar_url, last_seen_at)
  - `groups` (email/name, source) + `group_members`
  - `api_keys` (id, name, key_hash, created_by, created_at, last_used_at)
- [x] Implement schema migrations via `golang-migrate/migrate` with embedded
  versioned `.sql` files (`internal/db/migrations/`).
- [x] Repository interfaces + implementations:
  - `LinkRepo`: Create, Get, Update, Delete, List (paginated + sorted),
    Search, IncrementUseCount (async/buffered — see scalability note).
  - `UserRepo`: Upsert, Get, List.
  - `APIKeyRepo`: Create, Revoke, Validate.
- [x] Unit-test repositories against SQLite in-memory.
- [x] **Commit**: `phase 2: database layer`

---

## Phase 3 — Core redirect engine

- [x] Implement simple redirect: append path suffix / fragment to target.
- [x] Implement advanced redirect: Go-template engine with custom functions
  (`match`, `extract`, `replace`) and template variables (`path`, `parts`,
  `args`, `ua`, `email`).
- [x] Validate advanced templates at creation time.
- [x] Unit-test both modes with table-driven tests.
- [x] **Commit**: `phase 3: redirect engine`

---

## Phase 4 — HTTP server skeleton + domain redirect middleware

- [x] Wire up `chi` router.
- [x] Implement canonical-domain + HTTPS redirect middleware (skip for
  direct redirect requests).
- [x] Implement per-request structured logging middleware.
- [x] Serve `go/linkname[/...]` redirect route (no auth yet).
- [x] Add health-check endpoint `GET /healthz`.
- [x] Integration-test: server starts, redirect route returns 301/302.
- [x] **Commit**: `phase 4: HTTP server + domain middleware`

---

## Phase 5 — Authentication

- [ ] Tailscale auth: read `Tailscale-User-*` headers; populate request
  context with user identity.
- [ ] OIDC auth (`coreos/go-oidc` v3 + `golang.org/x/oauth2`): implement
  login/callback/logout routes; issue a signed JWT (`golang-jwt/jwt` v5)
  stored in a `Secure`/`HttpOnly`/`SameSite=Lax` cookie; fetch `email`,
  `name`, `picture`, `groups` claims.
- [ ] Auth middleware: attach identity to context; enforce
  `require_auth_for_redirects` when configured.
- [ ] `require_auth` per-link enforcement (redirect to auth first using
  canonical domain if OIDC).
- [ ] Upsert user record on successful auth.
- [ ] Unit-test Tailscale header parsing; integration-test OIDC flow with a
  mock provider.
- [ ] **Commit**: `phase 5: authentication`

---

## Phase 6 — Link management UI

- [ ] HTML templates (server-side `html/template` + HTMX + Bulma CSS):
  - Landing page: quick-create button, search box, recent links (owner),
    popular links.
  - `/new` — create link form (name, target, is_advanced, require_auth).
  - `/edit/<name>` — edit link; sharing UI (add/remove emails & groups).
  - `/links` — paginated, sortable full list of all links.
  - `/mylinks` — paginated, sortable list of owner's links.
  - `/help` — redirect pattern documentation.
- [ ] Implement quick-link random name generator.
- [ ] Enforce forbidden names (reserved endpoints).
- [ ] Enforce link name character and case rules.
- [ ] CSRF protection on all mutating forms.
- [ ] **Commit**: `phase 6: link management UI`

---

## Phase 7 — REST API

- [ ] Content-negotiate JSON vs HTML on existing routes.
- [ ] API key authentication middleware (Bearer / `X-API-Key`).
- [ ] Implement endpoints:
  - `POST /api/links` — create
  - `GET /api/links/:name` — resolve / get
  - `PATCH /api/links/:name` — update (field mask via JSON body)
  - `DELETE /api/links/:name` — delete
  - `GET /api/links` — list (paginated)
  - `POST /api/import` — bulk import
  - `GET /api/export` — full export
- [ ] Admin-only: `/apikeys` page + API (`GET/POST/DELETE /api/apikeys`).
- [ ] Write `docs/api.md` documenting every endpoint.
- [ ] Unit/integration tests for each endpoint.
- [ ] **Commit**: `phase 7: REST API`

---

## Phase 8 — Import / export

- [ ] JSON export: stream full DB dump (links + shares); admin only.
- [ ] JSON import: validate + upsert; admin only; return a summary report.
- [ ] Ensure both work via UI and API.
- [ ] Test round-trip: export → import → export produces identical output.
- [ ] **Commit**: `phase 8: import/export`

---

## Phase 9 — Scalability hardening

- [ ] Audit all DB queries for missing indices; add them.
- [ ] Make `IncrementUseCount` non-blocking: batch updates via a background
  goroutine with a ticker + channel, draining on shutdown.
- [ ] Add an in-process LRU cache for redirect lookups (hot links); size
  configurable; invalidate on edit/delete.
- [ ] Load-test or benchmark redirect path; document results.
- [ ] **Commit**: `phase 9: scalability hardening`

---

## Phase 10 — Documentation

- [ ] Complete `README.md` (overview, features, quick-start).
- [ ] Write `docs/deployment.md` (bare metal, Docker, Compose, Kubernetes).
- [ ] Write (or auto-generate) `docs/configuration.md` from
  `config.template.toml`.
- [ ] Write `docs/links.md` (redirect pattern help); reuse as `/help` page.
- [ ] Verify all doc links and code examples are correct.
- [ ] **Commit**: `phase 10: documentation`

---

## Phase 11 — Security review

- [ ] Audit all user-supplied inputs for injection vectors.
- [ ] Verify open-redirect protection (stored URLs only; reject `javascript:`,
  `data:`, and relative-path-only targets that could be abused).
- [ ] Confirm cookie flags: `Secure`, `HttpOnly`, `SameSite=Lax`.
- [ ] Confirm API keys are stored only as hashes.
- [ ] Run `go vet`, `staticcheck`, and `gosec`; fix all findings.
- [ ] **Commit**: `phase 11: security review`

---

## Phase 12 — CI / CD

- [ ] `.github/workflows/test.yml`: run `go test ./...` on PRs targeting
  `main`.
- [ ] `.github/workflows/docker.yml`:
  - Build image on push to `main` (cache layers).
  - Publish to `ghcr.io/mkende/golink-redirector` on `vX.Y.Z` tags
    (reuse cached layers from the main-push job).
- [ ] Add `Dockerfile` (multi-stage: builder + minimal runtime image).
- [ ] Add `docker-compose.yml` example.
- [ ] **Commit**: `phase 12: CI/CD`

---

## Phase 13 — Polish & release prep

- [ ] Final pass on all HTML templates: accessibility, mobile layout.
- [ ] Default favicon embedded in binary.
- [ ] Verify `config.template.toml` is complete and matches the live Config
  struct.
- [ ] Tag `v0.1.0` once all phases are complete and CI is green.

---

## Notes

- Phases 0–3 can be done without a running server; they are pure logic.
- Phases 4–8 build on each other sequentially.
- Phases 9–13 are hardening / finishing work and may partially overlap.
- **Ask before** any dimensioning decision listed in `AGENTS.md`.
