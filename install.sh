#!/usr/bin/env bash
set -euo pipefail

# curl -fsSL https://sphragis.eu/install.sh | bash
# Downloads a prebuilt sphragis binary for your OS/arch, or builds from source.

REPO="${SPHRAGIS_REPO:-sphragis-oss/sphragis}"
BINDIR="${SPHRAGIS_BINDIR:-/usr/local/bin}"

say() { printf '%s\n' "$*"; }
err() {
	printf 'error: %s\n' "$*" >&2
	exit 1
}

os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m)
case "$os" in linux | darwin) ;; *) err "unsupported OS: $os" ;; esac
case "$arch" in
x86_64 | amd64) arch=amd64 ;;
aarch64 | arm64) arch=arm64 ;;
*) err "unsupported arch: $arch" ;;
esac

put() {
	mkdir -p "$BINDIR" 2>/dev/null || true
	if [ -w "$BINDIR" ]; then
		install -m 0755 "$1" "$BINDIR/sphragis"
	else
		say "installing to $BINDIR (sudo may be requested)..."
		sudo install -d "$BINDIR"
		sudo install -m 0755 "$1" "$BINDIR/sphragis"
	fi
}

ver="${SPHRAGIS_VERSION:-}"
if [ -z "$ver" ]; then
	ver=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null |
		grep -m1 '"tag_name"' | cut -d'"' -f4 || true)
fi

if [ -n "$ver" ]; then
	url="https://github.com/${REPO}/releases/download/${ver}/sphragis_${os}_${arch}.tar.gz"
	tmp=$(mktemp -d)
	trap 'rm -rf "$tmp"' EXIT
	say "downloading sphragis ${ver} (${os}/${arch})..."
	if curl -fsSL "$url" -o "$tmp/s.tar.gz"; then
		tar -xzf "$tmp/s.tar.gz" -C "$tmp"
		put "$tmp/sphragis"
		say "installed: $BINDIR/sphragis"
		say "next: export SPHRAGIS_UPSTREAM_API_KEY=sk-... && sphragis start"
		exit 0
	fi
	say "no prebuilt binary for ${os}/${arch} at ${ver}; trying source build..."
fi

if command -v go >/dev/null 2>&1; then
	say "building from source via go install..."
	go install "github.com/${REPO}/cmd/sphragis@latest"
	say "installed to $(go env GOPATH)/bin/sphragis (ensure it is on PATH)"
	exit 0
fi

err "no published release for ${os}/${arch} and Go is not installed"
