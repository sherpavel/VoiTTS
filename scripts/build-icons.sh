#!/usr/bin/env bash
#
# THIS IS AI GENERATED, PROCEED WITH CAUTION
#
# Renders every icon this project ships from one SVG master, and writes each
# into the directory that consumes it:
#
#   assets/icons/    the launcher's icon. build-release.sh stages this into the
#                    tarball, install.sh installs it into the hicolor theme.
#   webui/static/    the web icons. adapter-static copies them to the dist root
#                    and internal/web compiles them into the binary.
#
# Only those two directories are written to, and the source SVG is only read.
# Re-running overwrites what it made last time, so this is the way to change
# the icon: edit the master, run this, commit the result.
#
# Two of the outputs are not plain renders, and both are wrong if you skip the
# extra step:
#
#   apple-touch-icon  iOS composites transparency to black, so a tile with
#                     rounded corners gets black triangles unless it is
#                     flattened first. iOS also masks the icon itself, so the
#                     corners must be square going in or they get rounded twice.
#
#   maskable          Android crops adaptive icons to a circle 80% of the
#                     canvas. The artwork has to be measured and scaled to fit
#                     inside that circle -- a fixed ratio only ever suits the
#                     one drawing it was picked for. That measurement is why
#                     ImageMagick is needed and rsvg-convert alone will not do.

set -euo pipefail

# The safe-circle arithmetic is the only floating point here, and it passes
# through awk and printf, both of which read and write the decimal separator of
# the locale. Under a comma locale that turns 0.8 into 0 and rejects 409.6.
export LC_ALL=C

readonly NAME="voitts"

readonly DESKTOP_SIZES=(16 24 32 48 64 128 256)
readonly ICO_SIZES=(16 32 48)
readonly APPLE_SIZE=180
readonly MANIFEST_SIZES=(192 512)
readonly MASKABLE_SIZE=512
# Android guarantees a circle of this fraction of the canvas is never cropped.
readonly MASKABLE_SAFE=0.8

# ImageMagick stamps date:create and date:modify into every PNG it writes, so
# an unchanged icon still comes out as different bytes on every run and shows
# up as a modified binary in git. rsvg-convert writes no such chunk, so only
# the composited icons need this.
readonly PNG_REPRODUCIBLE=(-define png:exclude-chunk=date)

readonly ASSETS="assets/icons"
readonly STATIC="webui/static"

dry_run=0
im=""
tmp=""

usage() {
	cat <<'USAGE'
Usage: build-icons.sh [-n] [-h] <master.svg>

Renders the launcher and web icons from one SVG and writes them into
assets/icons/ and webui/static/, overwriting what was there.

  -n  print the plan and exit, writing nothing into the project
  -h  show this text

Needs rsvg-convert and ImageMagick. -n still renders into a temporary
directory, so the sizes it prints are the ones it would write.
USAGE
}

die() {
	printf 'build-icons: %s\n' "$*" >&2
	exit 1
}

have() {
	command -v "$1" >/dev/null 2>&1
}

# run prints a command and runs it, or only prints it under -n.
run() {
	printf '  %s\n' "$*"
	if [ "$dry_run" -eq 0 ]; then
		"$@"
	fi
}

# ---------------------------------------------------------------- detection

# family names the packaging conventions of this distro, from /etc/os-release,
# and is only used to spell out the command that installs the two renderers.
detect_family() {
	local id="" like=""
	if [ -r /etc/os-release ]; then
		# shellcheck source=/dev/null
		. /etc/os-release
		id="${ID:-}"
		like="${ID_LIKE:-}"
	fi

	case " $id $like " in
	*" arch "* | *" cachyos "* | *" manjaro "* | *" endeavouros "*) family=arch ;;
	*" debian "* | *" ubuntu "*) family=debian ;;
	*" fedora "* | *" rhel "* | *" centos "*) family=fedora ;;
	*) family=unknown ;;
	esac
}

# package prints the package providing a role on this distro. Empty when this
# script cannot name it.
package() {
	case "$family:$1" in
	arch:rsvg) echo librsvg ;;
	arch:magick) echo imagemagick ;;
	debian:rsvg) echo librsvg2-bin ;;
	debian:magick) echo imagemagick ;;
	fedora:rsvg) echo librsvg2-tools ;;
	fedora:magick) echo ImageMagick ;;
	esac
}

# install_hint prints the command that would install the named roles, or an
# empty string when the distro is one this script does not recognise.
install_hint() {
	local role names=()
	for role in "$@"; do
		local name
		name="$(package "$role")"
		if [ -z "$name" ]; then
			return
		fi
		names+=("$name")
	done

	case "$family" in
	arch) printf 'sudo pacman -S --needed %s\n' "${names[*]}" ;;
	debian) printf 'sudo apt install %s\n' "${names[*]}" ;;
	fedora) printf 'sudo dnf install %s\n' "${names[*]}" ;;
	esac
}

# require_tools resolves ImageMagick's entry point, which is `magick` in v7 and
# `convert` in v6, and refuses to start without both renderers rather than
# failing partway through with half the icons written.
require_tools() {
	local missing=() hint
	have rsvg-convert || missing+=(rsvg)

	if have magick; then
		im=magick
	elif have convert; then
		im=convert
	else
		missing+=(magick)
	fi

	if [ ${#missing[@]} -eq 0 ]; then
		return
	fi

	detect_family
	printf 'build-icons: missing %s\n' "${missing[*]}" >&2
	hint="$(install_hint "${missing[@]}")"
	if [ -n "$hint" ]; then
		printf '  install with: %s\n' "$hint" >&2
	fi
	exit 1
}

# ---------------------------------------------------------------- geometry

# background prints the tile's own background colour, sampled just inside the
# top edge. Rounded corners are transparent, so the corner pixel is no use, and
# the midpoint of an edge is background for any drawing that has one.
background() {
	local w
	w="$("$im" "$1" -format '%w' info:)"
	"$im" "$1" -format "%[pixel:p{$((w / 2)),3}]" info:
}

# mark_extent prints "W H" for the drawing inside the tile. The background is
# flattened in first so a transparent corner reads as background and the trim
# finds the artwork instead of the whole canvas.
mark_extent() {
	local png="$1" bg
	bg="$(background "$png")"
	"$im" "$png" -background "$bg" -alpha remove -alpha off -fuzz 1% -format '%@' info: |
		awk -F'[x+]' '{ print $1, $2 }'
}

# maskable_render prints the pixel size to render the master at so that the
# drawing's diagonal fits Android's safe circle once centred on the full
# canvas. Artwork already small enough is left alone rather than enlarged.
maskable_render() {
	local w="$1" h="$2"
	awk -v w="$w" -v h="$h" -v size="$MASKABLE_SIZE" -v safe="$MASKABLE_SAFE" '
		BEGIN {
			d = sqrt(w * w + h * h)
			if (d <= 0) { print size; exit }
			limit = size * safe
			s = (d > limit) ? limit / d : 1
			print int(size * s)
		}'
}

# ---------------------------------------------------------------- rendering

# copy_master puts the master where a consumer expects it. Being handed the
# master at its own destination is the normal case -- assets/icons/voitts.svg
# is where it lives -- so that is a no-op, not an error.
copy_master() {
	if [ "$(readlink -f "$1")" = "$(readlink -f "$2")" ]; then
		printf '  # %s is the master already\n' "$2"
		return
	fi
	run cp "$1" "$2"
}

# render writes into the project, so -n only prints it.
render() {
	run rsvg-convert -w "$2" -h "$2" "$1" -o "$3"
}

# render_tmp writes into the temporary directory, which is not the project, and
# always runs: under -n the maskable measurement is still made from a real
# render, so the size the plan prints is the size it would write.
render_tmp() {
	printf '  rsvg-convert -w %s -h %s %s -o %s\n' "$2" "$2" "$1" "$3"
	rsvg-convert -w "$2" -h "$2" "$1" -o "$3"
}

main() {
	local master="$1"

	require_tools

	[ -f "$master" ] || die "no such file: $master"
	master="$(readlink -f "$master")"

	# Everything is written relative to the repo root, whatever directory this
	# was invoked from.
	cd "$(dirname "$(readlink -f "$0")")/.."

	tmp="$(mktemp -d)"
	trap 'rm -rf "$tmp"' EXIT

	rsvg-convert -w 64 -h 64 "$master" -o "$tmp/probe.png" 2>/dev/null ||
		die "rsvg-convert cannot render $master -- is it an SVG?"

	if [ "$dry_run" -eq 0 ]; then
		mkdir -p "$ASSETS" "$STATIC"
	fi

	# -- the launcher's icon -------------------------------------------------
	# Sizes are the filenames install.sh reads, so this set is the whole
	# contract between the two scripts.
	printf '\ndesktop  %s/\n' "$ASSETS"
	copy_master "$master" "$ASSETS/$NAME.svg"
	local size
	for size in "${DESKTOP_SIZES[@]}"; do
		render "$master" "$size" "$ASSETS/$NAME-$size.png"
	done

	# -- favicon.ico ---------------------------------------------------------
	# One file holding three sizes. Browsers request /favicon.ico whether or
	# not anything links to it, and internal/web answers a miss with a 404.
	printf '\nfavicon  %s/favicon.ico\n' "$ASSETS"
	local ico=()
	for size in "${ICO_SIZES[@]}"; do
		render_tmp "$master" "$size" "$tmp/ico-$size.png"
		ico+=("$tmp/ico-$size.png")
	done
	run "$im" "${ico[@]}" "$ASSETS/favicon.ico"

	# -- the web icons -------------------------------------------------------
	printf '\nweb      %s/\n' "$STATIC"
	copy_master "$master" "$STATIC/favicon.svg"
	run cp "$ASSETS/favicon.ico" "$STATIC/favicon.ico"
	for size in "${MANIFEST_SIZES[@]}"; do
		render "$master" "$size" "$STATIC/icon-$size.png"
	done

	# iOS: opaque, and square-cornered because iOS applies its own mask.
	local bg
	bg="$(background "$tmp/probe.png")"
	render_tmp "$master" "$APPLE_SIZE" "$tmp/apple.png"
	run "$im" "$tmp/apple.png" -background "$bg" -alpha remove -alpha off \
		"${PNG_REPRODUCIBLE[@]}" "$STATIC/apple-touch-icon.png"

	# Android: measured against the safe circle, then centred full-bleed.
	render_tmp "$master" "$MASKABLE_SIZE" "$tmp/full.png"
	local w h render_at
	read -r w h < <(mark_extent "$tmp/full.png")
	render_at="$(maskable_render "$w" "$h")"
	printf '  # drawing is %sx%s of %s -- rendering at %s to clear the safe circle\n' \
		"$w" "$h" "$MASKABLE_SIZE" "$render_at"
	render_tmp "$master" "$render_at" "$tmp/inner.png"
	run "$im" "$tmp/inner.png" -background "$bg" -gravity center \
		-extent "${MASKABLE_SIZE}x${MASKABLE_SIZE}" -alpha remove -alpha off \
		"${PNG_REPRODUCIBLE[@]}" "$STATIC/icon-512-maskable.png"

	if [ "$dry_run" -eq 1 ]; then
		printf '\nnothing written (-n)\n'
		return
	fi

	verify
}

# ---------------------------------------------------------------- verify

# The two derived icons are the ones worth checking, because both fail
# silently: a maskable icon that overflows is only visibly wrong on a phone,
# and transparency left in either only shows up as black on a device.
verify() {
	local w h status=0

	read -r w h < <(mark_extent "$STATIC/icon-512-maskable.png")
	if ! awk -v w="$w" -v h="$h" -v size="$MASKABLE_SIZE" -v safe="$MASKABLE_SAFE" \
		'BEGIN { exit !(sqrt(w * w + h * h) <= size * safe) }'; then
		printf '  x maskable  %sx%s overflows the safe circle\n' "$w" "$h" >&2
		status=1
	else
		printf '\n  + maskable  drawing %sx%s inside a %spx safe circle\n' "$w" "$h" \
			"$(awk -v s="$MASKABLE_SIZE" -v f="$MASKABLE_SAFE" 'BEGIN { printf "%.1f", s * f }')"
	fi

	local icon
	for icon in "$STATIC/apple-touch-icon.png" "$STATIC/icon-512-maskable.png"; do
		if [ "$("$im" "$icon" -format '%[opaque]' info:)" = "True" ]; then
			printf '  + opaque    %s\n' "$icon"
		else
			printf '  x opaque    %s still has transparency\n' "$icon" >&2
			status=1
		fi
	done

	[ "$status" -eq 0 ] || die "generated icons did not verify"
	printf '\nrun `pnpm --dir webui build` to pick the web icons up.\n'
}

while getopts ':nh' opt; do
	case "$opt" in
	n) dry_run=1 ;;
	h) usage; exit 0 ;;
	*) usage >&2; exit 2 ;;
	esac
done
shift $((OPTIND - 1))

if [ $# -ne 1 ]; then
	usage >&2
	exit 2
fi

main "$1"
