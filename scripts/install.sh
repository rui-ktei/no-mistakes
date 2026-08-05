#!/bin/sh
# Build-from-source installer: clones the repo, ensures a Go toolchain is
# present, builds and installs the binary via `make install`, then applies
# the ticket_prefix_pattern convention - the same steps the
# install-no-mistakes agent skill performs, packaged as a plain shell script
# for anyone without a skill-aware coding agent.
set -e

REPO_URL="${NO_MISTAKES_REPO_URL:-https://github.com/kunchenguid/no-mistakes.git}"
REPO_REF="${NO_MISTAKES_REPO_REF:-main}"

for cmd in git make curl tar; do
  command -v "$cmd" >/dev/null 2>&1 || { echo "error: '$cmd' is required" >&2; exit 1; }
done

# go.mod pins a `go` directive with no `toolchain` line, so once any Go
# 1.21+ is present, GOTOOLCHAIN=auto (the default) fetches a newer toolchain
# on demand if the build needs one. We only need to install Go from scratch
# when it is missing entirely.
ensure_go() {
  if command -v go >/dev/null 2>&1; then
    return
  fi

  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  arch="$(uname -m)"
  case "$os" in
    darwin|linux) ;;
    *) echo "error: unsupported OS '$os' - install Go manually: https://go.dev/dl/" >&2; exit 1 ;;
  esac
  case "$arch" in
    x86_64) arch=amd64 ;;
    arm64|aarch64) arch=arm64 ;;
    *) echo "error: unsupported architecture '$arch' - install Go manually: https://go.dev/dl/" >&2; exit 1 ;;
  esac

  echo "Go not found - installing the latest Go into \$HOME/.local/go..."
  gover="$(curl -fsSL 'https://go.dev/VERSION?m=text' | head -n 1)"
  gotar="$(mktemp)"
  curl -fsSL "https://go.dev/dl/${gover}.${os}-${arch}.tar.gz" -o "$gotar"
  rm -rf "$HOME/.local/go"
  tar -C "$HOME/.local" -xzf "$gotar"
  rm -f "$gotar"

  mkdir -p "$HOME/.local/bin"
  ln -sf "$HOME/.local/go/bin/go" "$HOME/.local/bin/go"
  ln -sf "$HOME/.local/go/bin/gofmt" "$HOME/.local/bin/gofmt"
  PATH="$HOME/.local/bin:$PATH"
  export PATH

  case ":$PATH:" in
    *":$HOME/.local/bin:"*) ;;
    *) echo "note: add 'export PATH=\"\$HOME/.local/bin:\$PATH\"' to your shell profile" >&2 ;;
  esac
}

ensure_go

workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT

echo "Cloning ${REPO_URL} (${REPO_REF})..."
git clone --depth 1 --branch "$REPO_REF" "$REPO_URL" "$workdir/no-mistakes"

(cd "$workdir/no-mistakes" && make install)

gobin_dir="$(go env GOPATH)/bin"
case ":$PATH:" in
  *":$gobin_dir:"*) ;;
  *) echo "note: add '$gobin_dir' to your PATH so no-mistakes/nom resolve." >&2 ;;
esac

# make install already restarted the daemon, which writes the default
# global config (with a commented-out ticket_prefix_pattern example) if it
# doesn't exist yet. Turn that convention on.
nm_home="${NM_HOME:-$HOME/.no-mistakes}"
config="$nm_home/config.yaml"
if [ -f "$config" ]; then
  if grep -qE '^[[:space:]]*#?[[:space:]]*ticket_prefix_pattern:' "$config"; then
    sed -i.bak -E 's/^[[:space:]]*#?[[:space:]]*ticket_prefix_pattern:.*/ticket_prefix_pattern: WEB-\\d+/' "$config"
    rm -f "$config.bak"
  else
    printf '\nticket_prefix_pattern: WEB-\\d+\n' >> "$config"
  fi
else
  echo "note: $config not found after install - skipping ticket_prefix_pattern setup" >&2
fi

echo
echo "no-mistakes installed. Run 'no-mistakes doctor' to verify prerequisites."
