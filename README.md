# golink-redirector

A production-grade Go URL shortener in the [go-link](https://www.golinks.io/) style.

## Features

- **Simple redirects**: `go/docs` → your long URL, with path passthrough (`go/docs/api` → `https://docs.example.com/api`)
- **Advanced redirects**: Go template syntax with regexp functions (`match`, `extract`, `replace`) and variables (`path`, `parts`, `args`, `ua`, `email`)
- **Authentication**: Tailscale header-based or OIDC (Authelia, Keycloak, etc.)
- **Link sharing**: Share links with other users by email or group
- **REST API**: Full CRUD with API key authentication
- **Import / export**: JSON dump and restore (admin only)
- **High performance**: LRU cache + async use-count writes; ~6.5 µs/op on the redirect path
- **SQLite default, PostgreSQL optional**

## Quick start

### 1. Download or build

```bash
go install github.com/mkende/golink-redirector/cmd/golink@latest
```

### 2. Configure

```bash
cp config.template.toml simple.conf
# Edit simple.conf — at minimum set canonical_domain
```

### 3. Run

```bash
golink -config simple.conf
```

The server starts on `0.0.0.0:8080` by default. Point your DNS for `go.example.com` at the server and set `canonical_domain = "go.example.com"` in the config.

## Documentation

| Doc | Description |
|-----|-------------|
| [docs/deployment.md](docs/deployment.md) | Self-host, Docker, Compose, Kubernetes |
| [docs/configuration.md](docs/configuration.md) | All configuration options |
| [docs/api.md](docs/api.md) | REST API reference |
| [docs/links.md](docs/links.md) | Redirect pattern help |

## License

MIT — see [LICENSE.txt](LICENSE.txt).
