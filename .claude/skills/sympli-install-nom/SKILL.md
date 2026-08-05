---
name: sympli-install-nom
description: Install the no-mistakes CLI ("nom") locally for a Sympli dev from this cloned repo - ensures a recent Go toolchain is present, then builds and installs the binary via `make install`. Use when a Sympli dev asks to install no-mistakes/nom, set up the no-mistakes CLI locally, or get this repo's tool working on their machine.
---

# Install no-mistakes (nom) for Sympli devs

This repo builds a single Go binary. Installing it locally is two steps: make sure a
recent Go is present, then run `make install` from the repo root.

## 1. Check Go

```sh
go version
```

`go.mod` requires Go 1.25.0 or newer. This repo's only storage dependency is
`modernc.org/sqlite`, a pure-Go SQLite driver - there is no cgo/gcc requirement, so Go
is genuinely the only toolchain prerequisite.

If `go` is missing, or its version is older than `go.mod` requires, install the latest
Go without sudo (mirrors how Go is already set up in the Sympli devcontainer, at
`~/.local/go` with a symlink in `~/.local/bin`):

```sh
GOVER=$(curl -fsSL 'https://go.dev/VERSION?m=text' | head -1)
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m); case "$ARCH" in x86_64) ARCH=amd64 ;; aarch64|arm64) ARCH=arm64 ;; esac

curl -fsSL "https://go.dev/dl/${GOVER}.${OS}-${ARCH}.tar.gz" -o /tmp/go.tar.gz
rm -rf "$HOME/.local/go"
tar -C "$HOME/.local" -xzf /tmp/go.tar.gz

mkdir -p "$HOME/.local/bin"
ln -sf "$HOME/.local/go/bin/go" "$HOME/.local/bin/go"
ln -sf "$HOME/.local/go/bin/gofmt" "$HOME/.local/bin/gofmt"
```

Make sure `$HOME/.local/bin` is on `PATH` (add `export PATH="$HOME/.local/bin:$PATH"`
to the shell profile if `go version` still doesn't resolve in a new shell).

## 2. Install

From the repo root:

```sh
make install
```

This builds `bin/no-mistakes`, installs it plus a `nom` symlink to
`$(go env GOPATH)/bin`, and restarts the no-mistakes daemon so it picks up the new
binary - no separate daemon step needed.

Confirm `$(go env GOPATH)/bin` (usually `~/go/bin`) is on `PATH` so the bare
`no-mistakes`/`nom` commands resolve afterward.

## 3. Verify

```sh
no-mistakes --version
no-mistakes doctor
```

`doctor` checks native agents, ACP aliases, provider tools, and whether the configured
global runner can start a validation gate - see the [installation guide prerequisites](/no-mistakes/start-here/installation/#prerequisites)
for what else (an agent runner, `gh`/`glab`/etc for PRs and CI) a dev may still need.
