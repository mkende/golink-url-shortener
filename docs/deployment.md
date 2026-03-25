# Deployment Guide

This document describes how to deploy golink-redirector in various environments.

---

## Self-hosted (bare metal)

### Prerequisites

- Go 1.21 or later
- A reverse proxy for TLS termination (nginx, Caddy, Traefik, etc.)
- A DNS record for your chosen short-link domain (e.g. `go.example.com`)

### Build

```bash
git clone https://github.com/mkende/golink-redirector.git
cd golink-redirector
go build -o golink ./cmd/golink
```

Or install directly:

```bash
go install github.com/mkende/golink-redirector/cmd/golink@latest
```

### Configure

Copy and edit the config template:

```bash
cp config.template.toml /etc/golink/simple.conf
```

At minimum set `canonical_domain`:

```toml
canonical_domain = "go.example.com"
```

See [configuration.md](configuration.md) for all available options.

### Run

```bash
golink -config /etc/golink/simple.conf
```

The server listens on `0.0.0.0:8080` by default. Adjust `listen_addr` in the config to change the bind address or port.

### systemd service

Create `/etc/systemd/system/golink.service`:

```ini
[Unit]
Description=golink-redirector URL shortener
After=network.target

[Service]
Type=simple
User=golink
Group=golink
ExecStart=/usr/local/bin/golink -config /etc/golink/simple.conf
Restart=on-failure
RestartSec=5s

# Harden the service
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ReadWritePaths=/var/lib/golink

[Install]
WantedBy=multi-user.target
```

Enable and start:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now golink
sudo journalctl -u golink -f
```

### Reverse proxy (nginx)

golink-redirector does not terminate TLS itself. Place it behind a reverse proxy. Example nginx config:

```nginx
server {
    listen 443 ssl http2;
    server_name go.example.com;

    ssl_certificate     /etc/letsencrypt/live/go.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/go.example.com/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

Alternatively, use [Caddy](https://caddyserver.com/) which handles TLS automatically:

```
go.example.com {
    reverse_proxy 127.0.0.1:8080
}
```

---

## Docker

A pre-built image is published to `ghcr.io/mkende/golink-redirector`.

### Basic run

```bash
docker run -d \
  --name golink \
  -p 8080:8080 \
  -v /data/golink:/data \
  -e GOLINK_CONFIG=/data/simple.conf \
  ghcr.io/mkende/golink-redirector:latest
```

Mount a directory to `/data` and place your `simple.conf` there. The SQLite database file is also stored there by default (set `db.dsn = "/data/golink.db"` in your config).

### Config via environment

The container reads the config file path from the `GOLINK_CONFIG` environment variable (or the `-config` flag). There is no environment-variable substitution for individual config keys; use the TOML file for all settings.

### Volumes

| Mount path | Purpose |
|------------|---------|
| `/data` | Config file and SQLite database (persistent) |

---

## Docker Compose

A minimal Compose file with SQLite:

```yaml
services:
  golink:
    image: ghcr.io/mkende/golink-redirector:latest
    restart: unless-stopped
    ports:
      - "8080:8080"
    volumes:
      - golink_data:/data
    environment:
      GOLINK_CONFIG: /data/simple.conf

volumes:
  golink_data:
```

### With PostgreSQL

```yaml
services:
  golink:
    image: ghcr.io/mkende/golink-redirector:latest
    restart: unless-stopped
    ports:
      - "8080:8080"
    volumes:
      - ./simple.conf:/etc/golink/simple.conf:ro
    environment:
      GOLINK_CONFIG: /etc/golink/simple.conf
    depends_on:
      postgres:
        condition: service_healthy

  postgres:
    image: postgres:16-alpine
    restart: unless-stopped
    environment:
      POSTGRES_DB: golink
      POSTGRES_USER: golink
      POSTGRES_PASSWORD: changeme
    volumes:
      - pg_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U golink"]
      interval: 5s
      timeout: 5s
      retries: 5

volumes:
  pg_data:
```

Set the DSN in `simple.conf`:

```toml
[db]
driver = "postgres"
dsn = "host=postgres port=5432 dbname=golink user=golink password=changeme sslmode=disable"
```

### Mounting the config

When using bind mounts instead of named volumes, ensure the config file exists on the host before running `docker compose up`.

---

## Kubernetes

The examples below use a single-replica SQLite deployment for simplicity. For multi-replica deployments, switch to PostgreSQL (see the note at the end of this section).

### ConfigMap

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: golink-config
  namespace: golink
data:
  simple.conf: |
    listen_addr = "0.0.0.0:8080"
    canonical_domain = "go.example.com"
    title = "GoLink"

    [db]
    driver = "sqlite"
    dsn = "/data/golink.db"

    [oidc]
    enabled = true
    issuer = "https://auth.example.com"
    client_id = "golink"
    client_secret = "changeme"
    redirect_url = "https://go.example.com/auth/callback"
    jwt_secret = "replace-with-a-32-byte-random-string"

    admin_emails = ["admin@example.com"]
```

### PersistentVolumeClaim

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: golink-data
  namespace: golink
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
```

### Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: golink
  namespace: golink
spec:
  replicas: 1
  selector:
    matchLabels:
      app: golink
  template:
    metadata:
      labels:
        app: golink
    spec:
      containers:
        - name: golink
          image: ghcr.io/mkende/golink-redirector:latest
          args: ["-config", "/etc/golink/simple.conf"]
          ports:
            - containerPort: 8080
          volumeMounts:
            - name: config
              mountPath: /etc/golink
              readOnly: true
            - name: data
              mountPath: /data
          readinessProbe:
            httpGet:
              path: /healthz
              port: 8080
            initialDelaySeconds: 3
            periodSeconds: 10
          livenessProbe:
            httpGet:
              path: /healthz
              port: 8080
            initialDelaySeconds: 5
            periodSeconds: 30
      volumes:
        - name: config
          configMap:
            name: golink-config
        - name: data
          persistentVolumeClaim:
            claimName: golink-data
```

### Service and Ingress

```yaml
apiVersion: v1
kind: Service
metadata:
  name: golink
  namespace: golink
spec:
  selector:
    app: golink
  ports:
    - port: 80
      targetPort: 8080
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: golink
  namespace: golink
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-prod
spec:
  ingressClassName: nginx
  tls:
    - hosts:
        - go.example.com
      secretName: golink-tls
  rules:
    - host: go.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: golink
                port:
                  number: 80
```

### Multi-replica note

SQLite uses file-level locking and is not safe for concurrent writes from multiple processes. If you need more than one replica (for high availability or rolling updates), switch to PostgreSQL:

```toml
[db]
driver = "postgres"
dsn = "host=postgres.golink.svc.cluster.local dbname=golink user=golink password=changeme sslmode=require"
```

With PostgreSQL you can safely run multiple replicas. The in-process LRU cache is per-replica; cache invalidation on edit/delete applies only to the local replica, so there may be a brief window (at most one cache TTL) where a stale entry is served on other replicas.
