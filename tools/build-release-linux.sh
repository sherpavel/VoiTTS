#!/usr/bin/env bash
#
# THIS IS AI GENERATED, PROCEED WITH CAUTION
#
# Builds a voitts-server release tarball for linux/amd64 into out/, ready to
# upload to a GitHub release by hand.
#
# The web UI is compiled into the binary by internal/web, so the frontend is
# built first. That order matters: `go build` succeeds against an empty
# internal/web/dist/app and produces a server that then refuses to start, which
# is what the index.html check below is guarding against.
#
# CGO is off so the binary is static, and runs on machines whose libc differs
# from the one it was built against.
#
# install.sh and voitts.desktop go into the tarball beside the binary, so an
# unpacked release installs itself without a checkout.

set -euo pipefail

version="${1:-}"
if [ -z "$version" ]; then
	echo "usage: build-release-linux.sh <version>    e.g. build-release-linux.sh v0.1.0" >&2
	exit 2
fi

# Run from the repo root whatever directory this was invoked from.
cd "$(dirname "$(readlink -f "$0")")/.."

pnpm --dir webui install --frozen-lockfile
pnpm --dir webui build
[ -s internal/web/dist/app/index.html ] ||
	{ echo "build-release: pnpm build left nothing for internal/web to embed" >&2; exit 1; }

# Everything is staged under one directory so the archive unpacks into a
# folder of its own. A flat tarball spills five files -- README.md and LICENSE
# among them -- over whatever directory it is opened in, overwriting files
# that happen to share those names.
name="voitts-server_${version}_linux_amd64"
stage="out/$name"
rm -rf "$stage"
mkdir -p "$stage"

CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
	go build -ldflags="-s -w -X main.version=$version" -o "$stage/voitts-server" ./cmd/server

install -m755 assets/bundle/install.sh "$stage/install.sh"
install -m755 assets/bundle/install-piper.sh "$stage/install-piper.sh"
install -m644 assets/bundle/voitts.desktop README.md LICENSE "$stage/"

# The launcher's icon, in the layout install.sh expects. Only the desktop set:
# the web icons live in webui/static and are already inside the binary.
mkdir -p "$stage/icons"
install -m644 assets/icons/voitts.svg assets/icons/voitts-*.png "$stage/icons/"

tar -czf "out/$name.tar.gz" -C out "$name"
rm -rf "$stage"

ls -1sh out
