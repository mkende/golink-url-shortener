# GoLink URL shortener

High performance URL shortener, with support for Go templates, OIDC, tailscale,
etc.

## Features

- **Powerful redirection syntax**: [advanced syntax](docs/links.md) based on Go
  templates with regex functions and variables extracted from context (for
  example, include the current user email address in the destination URL). Also
  support a simpler syntax and explicit link aliases.
- **Authentication**: Multiple supported authentication source, such as
  Tailscale header-based, OIDC (Authelia, Keycloak, etc.) or reverse proxy
  autorization header. Link redirection can require authentication (server wide
  or per-link option).
- **Short domain redirect**: Support redirection and web-site access through
  short domains (such as go/foobar), typically through Tailscale MagicDNS.
- **Link sharing**: Share links ownership with other users by email or group.
- **Search**: Search link feature, with restrict.
- **REST API**: Full API support to create/edit/resolve links and perform
  administrative duties (DB dump and restore). With read-only or read/write API
  keys management.
- **Admin interface**: Full admin interface for management duty or override for
  user’s links.
- **Easy to deploy**: Just one database is required (SQLite or Postgres), follow
  our [guides](docs/deployment.md) for Docker, Docker compose, Kubernetes, etc.
- **High performance**: LRU cache and async use-count writes. Designed for 
  hundred of thousands of users and short links.
- **Security**: Configurable domain restriction for sharing, advanced links.
  Full CSP policy, CIDR checks for trusting headers, etc.

## Quick start

### 1. Download or build

**Install from source** (requires Go 1.21+):

```bash
go install github.com/mkende/golink-url-shortener/cmd/golink@latest
```

`@latest` resolves to the most recent tagged release. The Go toolchain embeds
that tag in the binary, so the footer shows the correct version automatically.

**Or build locally** (requires [just](https://github.com/casey/just)):

```bash
git clone https://github.com/mkende/golink-url-shortener.git
cd golink-url-shortener
just build        # build binary (version stamped from VERSION file)
just install      # install to $GOBIN / $GOPATH/bin
just test         # run all tests
just run          # build and run locally (requires config.toml)
just run-docker   # run in Docker (requires config.toml and just build-container)
```

Running `go build` directly produces a binary labelled `dev`.

### 2. Configure

```bash
cp config.template.toml golink.conf
# Edit golink.conf
```

### 3. Run

```bash
golink -config golink.conf
```

The server starts on `0.0.0.0:8080` by default.

## AI Usage disclosure

While the code itself of this server has been heavily written by Claude, the
interface, the code structure, the authentication flow, the feature set, etc.
have all been carefully designed and reviewed by a human. In addition,
exhaustive testing of the various deployment options took place.

## Documentation

- **[docs/deployment.md](docs/deployment.md)** — Self-host, Docker, Compose, Kubernetes
- **[docs/configuration.md](docs/configuration.md)** — All configuration options
- **[docs/api.md](docs/api.md)** — REST API reference
- **[docs/links.md](docs/links.md)** — Redirect pattern help
- **[docs/auth-redirect](docs/auth-redirect.md)** — Implementation details on
  the authentication patterns

## License

MIT — see [LICENSE.txt](LICENSE.txt).
