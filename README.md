# golink url shortener

A production-grade URL shortener in the *go/* link style.

## Features

- **Redirection syntax**: supports a simple syntax or advanced Go template with
  regexp functions (`match`, `extract`, `replace`) and variables (`path`,
  `parts`, `args`, `ua`, `email`)
- **Authentication**: Tailscale header-based, OIDC (Authelia, Keycloak, etc.),
  reverse proxy autorization header, etc.
- **Link sharing**: Share links with other users by email or group
- **REST API**: Full API support to create/edit/resolve links and perform
  administrative duties (DB dump and restore).
- **Easy to deploy**: Just one SQLite DB (or optionnaly Postgres), documentation
  for Docker, Docker compose, Kubernetes, etc.
- **High performance**: LRU cache and async use-count writes. Designed for 
  hundred of thousands of users and short links.

## Quick start

### 1. Download or build

**Install from source** (requires Go 1.21+):

```bash
go install github.com/mkende/golink-url-shortener/cmd/golink@latest
```

**Or build locally**:

```bash
git clone https://github.com/mkende/golink-url-shortener.git
cd golink-url-shortener
go build -o golink ./cmd/golink
```

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

## Documentation

| Doc | Description |
|-----|-------------|
| [docs/deployment.md](docs/deployment.md) | Self-host, Docker, Compose, Kubernetes |
| [docs/configuration.md](docs/configuration.md) | All configuration options |
| [docs/api.md](docs/api.md) | REST API reference |
| [docs/links.md](docs/links.md) | Redirect pattern help |

## License

MIT — see [LICENSE.txt](LICENSE.txt).
