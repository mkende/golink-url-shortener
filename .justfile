version := `cat VERSION`
image   := "golink"

# Run lint, check-format, and test (default target).
default: lint check-format test

# Run all tests.
test:
    go test ./...

# Run go vet and staticcheck.
lint:
    go vet ./...
    go tool staticcheck ./...

# Format all Go source files in place.
format:
    gofmt -w .

# Check formatting without modifying files; fails if any file is unformatted.
check-format:
    @test -z "$(gofmt -l .)" || \
        (echo "The following files are not formatted (run 'just format' to fix):" && \
         gofmt -l . && exit 1)

alias all := build

# Build the golink binary, stamping the version from the VERSION file.
build:
    go build -ldflags="-X github.com/mkende/golink-url-shortener/internal/version.Version=v{{version}}" \
        -o golink ./cmd/golink

# Install the binary into $GOBIN (or $GOPATH/bin).
install:
    go install -ldflags="-X github.com/mkende/golink-url-shortener/internal/version.Version=v{{version}}" \
        ./cmd/golink

# Remove the installed binary from $GOBIN (or $GOPATH/bin).
uninstall:
    #!/usr/bin/env bash
    set -euo pipefail
    bin=$(go env GOBIN)
    [[ -z "$bin" ]] && bin="$(go env GOPATH)/bin"
    rm -f "$bin/golink"
    echo "Removed $bin/golink"

# Build the Docker image locally, tagged as dev.
build-container:
    docker build --build-arg VERSION=v{{version}} -t {{image}}:dev .

# Tag the local dev image with the current version and latest.
tag-container:
    docker tag {{image}}:dev {{image}}:v{{version}}
    docker tag {{image}}:dev {{image}}:latest

# Run the local binary with config.toml (builds first).
run: build check-config
    ./golink -config config.toml

# Run the locally built Docker image with config.toml (builds the container first).
run-docker: build-container check-config
    docker run --rm \
        -v "$(pwd)/config.toml:/config/golink.conf:ro" \
        -e GOLINK_CONFIG=/config/golink.conf \
        -p 8080:8080 \
        {{image}}:dev

# Fail with a clear message if config.toml is missing.
[private]
check-config:
    @test -f config.toml || \
        (echo "error: config.toml not found — copy config.template.toml and edit it" >&2 && exit 1)
