#!/usr/bin/env bash
#
# THIS IS AI GENERATED, PROCEED WITH CAUTION
#
# Installs voitts-server and its launcher out of an unpacked release tarball.
# Everything lands in your home directory: no root, no system packages, and
# nothing the runtime needs -- Piper, the voice model and the audio stack are
# install-piper.sh's job, and the server checks for them itself at startup.
#
# The launcher's Exec has to name the binary by absolute path, because the
# PATH a GUI session hands its children is not the one your shell builds and
# does not include ~/.local/bin. That is what the __BIN__ substitution is for.

set -euo pipefail

here="$(dirname "$(readlink -f "$0")")"

bindir="${BINDIR:-$HOME/.local/bin}"
appdir="${APPDIR:-$HOME/.local/share/applications}"
# Icon lookup always searches $XDG_DATA_HOME/icons, which is this by default --
# so `Icon=voitts` in the .desktop resolves without touching anything system
# wide. Overriding this to a directory outside that search path installs the
# files but leaves the launcher iconless.
icondir="${ICONDIR:-$HOME/.local/share/icons}"

# Next to this script in an unpacked tarball; in out/ when run from a checkout
# that has just been through build-release.sh.
binary="$here/voitts-server"
[ -f "$binary" ] || binary="$here/../../out/voitts-server"
if [ ! -f "$binary" ]; then
	echo "install: no voitts-server beside this script or in out/" >&2
	echo "         unpack the release tarball first, or run build-release-linux.sh" >&2
	exit 1
fi

desktop="$here/voitts.desktop"
[ -f "$desktop" ] || { echo "install: voitts.desktop is missing from $here" >&2; exit 1; }

# Same two places as the binary: beside this script in an unpacked tarball,
# and up two from assets/bundle/ to assets/icons/ when run from a checkout.
icons="$here/icons"
[ -d "$icons" ] || icons="$here/../../assets/icons"
if [ ! -f "$icons/voitts.svg" ]; then
	echo "install: no icons beside this script or in assets/icons" >&2
	exit 1
fi

mkdir -p "$bindir" "$appdir"
install -m755 "$binary" "$bindir/voitts-server"
# The launcher runs with the session PATH, which does not include ~/.local/bin.
# The server needs it there to find Piper: `uv tool install piper-tts` puts the
# executable in ~/.local/bin, so without this the launcher starts a server that
# fails its own preflight while the same binary works fine from a terminal.
pathprefix="$HOME/.local/bin"
[ "$bindir" = "$pathprefix" ] || pathprefix="$bindir:$pathprefix"

# Only on the Exec line, so a comment naming a placeholder is left alone.
sed "/^Exec=/ {
	s|__PATH__|$pathprefix|
	s|__BIN__|$bindir/voitts-server|
}" "$desktop" > "$appdir/voitts.desktop"

# Into the hicolor theme, which is the fallback every desktop searches last, so
# a themed name resolves no matter which icon theme the user runs. Sizes come
# from the filenames rather than a hardcoded list: dropping a voitts-512.png
# into assets/icons is enough to ship it.
install -Dm644 "$icons/voitts.svg" "$icondir/hicolor/scalable/apps/voitts.svg"
pngs=0
for png in "$icons"/voitts-*.png; do
	[ -f "$png" ] || continue
	size="${png##*/voitts-}"
	size="${size%.png}"
	case "$size" in *[!0-9]*|"") continue ;; esac
	install -Dm644 "$png" "$icondir/hicolor/${size}x${size}/apps/voitts.png"
	pngs=$((pngs + 1))
done

# Refreshes the application menu. Absent on some systems, and only a cache.
if command -v update-desktop-database >/dev/null; then
	update-desktop-database "$appdir" || true
fi

# Likewise a cache. -t is what makes it work here: this directory has no
# index.theme and no reason to carry one, and the call would fail without it.
if command -v gtk-update-icon-cache >/dev/null; then
	gtk-update-icon-cache -qft "$icondir/hicolor" 2>/dev/null || true
fi

echo "installed  $bindir/voitts-server"
echo "           $appdir/voitts.desktop"
echo "           $icondir/hicolor  (voitts.svg + $pngs png sizes)"
echo
echo "VoiTTS is now in your application menu."

case ":$PATH:" in
	*":$bindir:"*) ;;
	*) echo; echo "note: $bindir is not on your PATH -- the launcher works, but the" >&2
	   echo "      voitts-server command will not until you add it" >&2 ;;
esac
