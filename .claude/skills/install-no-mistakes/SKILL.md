---
name: install-no-mistakes
description: Install the no-mistakes CLI ("nom") locally from this cloned repo - ensures a recent Go toolchain is present, builds and installs the binary via `make install`, and sets the org-wide ticket_prefix_pattern config convention. Use when a dev asks to install no-mistakes/nom, set up the no-mistakes CLI locally, or get this repo's tool working on their machine.
---

# Install no-mistakes (nom)

This repo builds a single Go binary. Installing it locally takes three steps: make
sure a recent Go is present, run `make install` from the repo root, then set the
org's ticket-prefix convention in the global config.

## 1. Check Go

```sh
go version
```

`go.mod` requires Go 1.25.0 or newer. This repo's only storage dependency is
`modernc.org/sqlite`, a pure-Go SQLite driver - there is no cgo/gcc requirement, so Go
is genuinely the only toolchain prerequisite.

If `go` is missing, or its version is older than `go.mod` requires, install the latest
Go without sudo, into a user-writable location:

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

## 3. Set the ticket-prefix convention

Restarting the daemon (the last thing `make install` does) materializes the global
config file at `$NM_HOME/config.yaml` (default `~/.no-mistakes/config.yaml`) if it
doesn't exist yet, pre-populated with a commented-out example:

```yaml
# ticket_prefix_pattern: 'WEB-\d+'
```

Uncomment it (or add the line if it's somehow missing) so it reads:

```yaml
ticket_prefix_pattern: 'WEB-\d+'
```

This makes every gated repo extract a `WEB-12345`-style ticket id from the branch
name (falling back to the PR title, then the oldest authored commit subject) and
prepend it to PR titles and gate-authored commit subjects, instead of plain
conventional-commit formatting. A repo can still override this locally by setting
its own non-empty `ticket_prefix_pattern` in its `.no-mistakes.yaml`.

## 4. Verify

```sh
no-mistakes --version
no-mistakes doctor
```

`doctor` checks native agents, ACP aliases, provider tools, and whether the configured
global runner can start a validation gate - see the [installation guide prerequisites](/no-mistakes/start-here/installation/#prerequisites)
for what else (an agent runner, `gh`/`glab`/etc for PRs and CI) a dev may still need.
