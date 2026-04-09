# GoLink URL shortener

High performance URL shortener, with support for Go templates, OIDC, tailscale,
etc.

## Features

- **Redirection syntax**: supports a simple syntax or an
  [advanced syntax](docs/links.md) based on Go templates with regex functions
  and variables extracted from context (for example, include the current user
  email address in the destination URL).
- **Authentication**: Tailscale header-based, OIDC (Authelia, Keycloak, etc.) or
  reverse proxy autorization header.
- **Link sharing**: Share links ownership with other users by email or group.
- **Easy to deploy**: Just one SQLite DB (or, optionnaly, Postgres), follow our
  [guides](docs/deployment.md) for Docker, Docker compose, Kubernetes, etc.
- **High performance**: LRU cache and async use-count writes. Designed for 
  hundred of thousands of users and short links.
- **REST API**: Full API support to create/edit/resolve links and perform
  administrative duties (DB dump and restore).

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
