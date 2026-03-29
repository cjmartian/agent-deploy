# Distribution Specification

## Overview

The `agent-deploy` MCP server currently only works when applications are co-located in the same repository. To use it from a separate Codespace or machine — e.g. a repo containing only portfolio, tic-tac-toe, and timeservice — the binary must be installable as a standalone tool. This spec covers restructuring the Go module for `go install`, publishing release binaries via GitHub Actions, and documenting MCP client setup for external repos.

## Current State

| Issue | Detail |
|-------|--------|
| Entry point lives under `internal/` | Go enforces that `internal/` packages cannot be imported by external modules, so `go install github.com/cjmartian/agent-deploy/internal@latest` **does not work** |
| No release binaries | There are no prebuilt binaries attached to GitHub releases |
| No install documentation | README shows how to build from source inside the repo but not how to install externally |

## Requirements

### 1. Restructure Entry Point for `go install`

Move the main package from `internal/main.go` to `cmd/agent-deploy/main.go` so that external `go install` works.

**File moves:**

| From | To |
|------|-----|
| `internal/main.go` | `cmd/agent-deploy/main.go` |
| `internal/main_test.go` | `cmd/agent-deploy/main_test.go` |

The `internal/` directory continues to hold all library packages (`awsclient`, `providers`, `state`, etc.) — those remain unexportable by design.

**Updated package declaration and imports in `cmd/agent-deploy/main.go`:**

```go
package main

import (
	"github.com/cjmartian/agent-deploy/internal/awsclient"
	"github.com/cjmartian/agent-deploy/internal/logging"
	"github.com/cjmartian/agent-deploy/internal/providers"
	"github.com/cjmartian/agent-deploy/internal/spending"
	"github.com/cjmartian/agent-deploy/internal/state"
	// ...
)
```

Note: `cmd/agent-deploy/` is *within* the same module, so importing `internal/` sub-packages is permitted.

**Updated Makefile targets:**

```makefile
build:
	go build -ldflags "-X main.Version=$(VERSION)" -o agent-deploy ./cmd/agent-deploy

install:
	go install -ldflags "-X main.Version=$(VERSION)" ./cmd/agent-deploy

run: build
	./agent-deploy
```

### 2. GitHub Releases with Prebuilt Binaries

Add a GitHub Actions workflow that builds and publishes cross-platform binaries on tagged releases using `goreleaser`.

**File:** `.github/workflows/release.yml`

```yaml
name: Release

on:
  push:
    tags:
      - "v*"

permissions:
  contents: write

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod

      - uses: goreleaser/goreleaser-action@v6
        with:
          version: latest
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

**File:** `.goreleaser.yml`

```yaml
version: 2

builds:
  - main: ./cmd/agent-deploy
    binary: agent-deploy
    ldflags:
      - -X main.Version={{ .Version }}
    goos:
      - linux
      - darwin
    goarch:
      - amd64
      - arm64

archives:
  - formats: [tar.gz]
    name_template: "{{ .ProjectName }}_{{ .Os }}_{{ .Arch }}"

checksum:
  name_template: "checksums.txt"

release:
  github:
    owner: cjmartian
    name: agent-deploy
```

**Target platforms:**

| OS | Arch | Notes |
|----|------|-------|
| linux | amd64 | GitHub Codespaces, most CI |
| linux | arm64 | Graviton-based Codespaces |
| darwin | amd64 | Intel Macs |
| darwin | arm64 | Apple Silicon Macs |

### 3. Installation Methods

After the restructuring and release workflow are in place, three installation methods are available:

#### a) `go install` (requires Go)

```sh
go install github.com/cjmartian/agent-deploy/cmd/agent-deploy@latest
```

#### b) Download release binary (no Go required)

```sh
curl -sL https://github.com/cjmartian/agent-deploy/releases/latest/download/agent-deploy_linux_amd64.tar.gz \
  | tar xz -C /usr/local/bin agent-deploy
```

#### c) Devcontainer feature / postCreateCommand

In a consuming repo's `.devcontainer/devcontainer.json`:

```jsonc
{
  "postCreateCommand": "go install github.com/cjmartian/agent-deploy/cmd/agent-deploy@latest"
}
```

Or with a pinned release binary:

```jsonc
{
  "postCreateCommand": "curl -sL https://github.com/cjmartian/agent-deploy/releases/latest/download/agent-deploy_linux_amd64.tar.gz | sudo tar xz -C /usr/local/bin agent-deploy"
}
```

### 4. MCP Client Configuration for External Repos

The consuming repo should include MCP server configuration so the agent can use `agent-deploy` tools automatically.

**File:** `.vscode/settings.json` (in the consuming repo)

```jsonc
{
  "mcp": {
    "servers": {
      "agent-deploy": {
        "command": "agent-deploy"
      }
    }
  }
}
```

Alternatively, `.vscode/mcp.json`:

```jsonc
{
  "servers": {
    "agent-deploy": {
      "command": "agent-deploy",
      "env": {
        "AWS_REGION": "us-east-1"
      }
    }
  }
}
```

AWS credentials should be provided via environment variables (`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`) set in the Codespace secrets, not committed to the repo.

### 5. README Updates

Add an **Installation** section to `README.md` covering `go install`, release binary download, and devcontainer setup. Add a **Using from another repository** section showing the MCP client configuration.

## Implementation Order

1. Move `internal/main.go` → `cmd/agent-deploy/main.go` (and the test file)
2. Update Makefile `build`, `install`, and `run` targets
3. Verify `go build ./cmd/agent-deploy` and `go test ./...` still pass
4. Add `.goreleaser.yml`
5. Add `.github/workflows/release.yml`
6. Update `README.md` with installation and external-repo usage instructions
7. Tag `v0.1.0` to trigger the first release

## Notes

- The `internal/` package restriction only applies to *external* modules. Since `cmd/agent-deploy/` lives inside the same `github.com/cjmartian/agent-deploy` module, it can import `internal/` sub-packages without issue.
- Windows is omitted from release targets since the server relies on Docker and AWS CLI patterns that are predominantly Linux/macOS. It can be added later if needed.
- Existing CI workflow (if any) should update its build path from `./internal` to `./cmd/agent-deploy`.
