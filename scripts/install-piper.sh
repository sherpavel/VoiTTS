#!/usr/bin/env bash
#
# THIS IS AI GENERATED, PROCEED WITH CAUTION
#
# Installs the Piper half of what voitts-server needs at runtime: the
# synthesizer and the voice it speaks with, both into your home directory with
# uv or pipx.
#
# It never installs system packages and never asks for root. The audio stack —
# pactl and a PCM player — is reported when missing, along with the command
# that would install it, and left for you to run: those packages sit under the
# sound server the rest of your desktop is using, and that is not a decision
# for a setup script.
#
# The checks mirror the ones cmd/server/check.go runs at startup, so what this
# script calls missing is what the server will refuse to start without.

set -euo pipefail

readonly VOICE="en_US-hfc_male-medium"
readonly VOICE_BASE_URL="https://huggingface.co/rhasspy/piper-voices/resolve/main/en/en_US/hfc_male/medium"
readonly VOICE_DIR="${XDG_DATA_HOME:-$HOME/.local/share}/piper/voices"

assume_yes=0
dry_run=0

usage() {
	cat <<'USAGE'
Usage: install-piper.sh [-y] [-n] [-h]

Installs Piper and the default voice model into your home directory, with uv
or pipx. System packages are never installed: a missing audio stack is
reported with the command for your distro, and left to you.

  -y  do not ask before installing
  -n  print the plan and exit, changing nothing
  -h  show this text

Exits non-zero when something it does not install is still missing.
USAGE
}

die() {
	printf 'install-piper: %s\n' "$*" >&2
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

# family names the packaging conventions of this distro, from /etc/os-release.
# Only used to spell the audio-stack command out for you.
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

# package prints the package providing a role on this distro, matching the
# table in README.md. Empty when this script cannot name it.
package() {
	case "$family:$1" in
	arch:pactl) echo libpulse ;;
	arch:server) echo pipewire-pulse ;;
	arch:player) echo pipewire-audio ;;
	debian:pactl) echo pulseaudio-utils ;;
	debian:server) echo pipewire-pulse ;;
	debian:player) echo pipewire-bin ;;
	fedora:pactl) echo pulseaudio-utils ;;
	fedora:server) echo pipewire-pulseaudio ;;
	fedora:player) echo pipewire-utils ;;
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

# piper_present mirrors resolvePiperCmd: the Python module first, because the
# distro package named `piper` on Arch is a mouse configurator.
piper_present() {
	local py
	for py in python3 python; do
		if have "$py" && "$py" -c 'import piper.__main__' >/dev/null 2>&1; then
			return 0
		fi
	done
	have piper
}

# voice_dirs mirrors piperDataDirs, most specific first.
voice_dirs() {
	if [ -n "${XDG_DATA_HOME:-}" ]; then
		printf '%s\n%s\n' "$XDG_DATA_HOME/piper/voices" "$XDG_DATA_HOME/piper"
	fi
	printf '%s\n%s\n%s\n%s\n%s\n' \
		"$HOME/.local/share/piper/voices" "$HOME/.local/share/piper" \
		"$PWD" /usr/share/piper/voices /usr/share/piper
}

# voice_present wants both files: the model, and the config holding the sample
# rate the whole pipeline is built from.
voice_present() {
	local dir
	while IFS= read -r dir; do
		if [ -f "$dir/$VOICE.onnx" ] && [ -f "$dir/$VOICE.onnx.json" ]; then
			return 0
		fi
	done < <(voice_dirs)
	return 1
}

# -------------------------------------------------------------------- plan

missing_audio=() # roles, in the order install_hint should name them
need_pactl=0
need_player=0
install_piper=0
install_voice=0

collect() {
	if ! have pactl; then
		need_pactl=1
		missing_audio+=(pactl server)
	fi
	if ! have pw-cat && ! have paplay; then
		need_player=1
		missing_audio+=(player)
	fi
	if ! piper_present; then
		install_piper=1
	fi
	if ! voice_present; then
		install_voice=1
	fi
}

# describe_audio prints what this script will not touch, and how to install it.
describe_audio() {
	if [ "${#missing_audio[@]}" -eq 0 ]; then
		return
	fi

	printf 'Missing, and left to you — this script installs no system packages:\n\n'
	if [ "$need_pactl" -eq 1 ] && [ "$need_player" -eq 1 ]; then
		printf '  the audio stack: pactl, and pw-cat or paplay\n'
	elif [ "$need_pactl" -eq 1 ]; then
		printf '  pactl, which loads the null sink and the remapped source\n'
	else
		printf '  pw-cat or paplay, which push PCM into the sink\n'
	fi

	local hint
	hint="$(install_hint "${missing_audio[@]}")"
	if [ -n "$hint" ]; then
		printf '    %s\n' "$hint"
	else
		printf '    see the package table in README.md for your distro\n'
	fi
	printf '\n'
}

describe_piper() {
	if [ "$install_piper" -eq 0 ] && [ "$install_voice" -eq 0 ]; then
		return
	fi

	printf 'Will be installed into your home directory:\n\n'
	if [ "$install_piper" -eq 1 ]; then
		if have uv; then
			printf '  piper      uv tool install piper-tts\n'
		else
			printf '  piper      pipx install piper-tts\n'
		fi
	fi
	if [ "$install_voice" -eq 1 ]; then
		printf '  voice      %s into %s\n' "$VOICE" "$VOICE_DIR"
	fi
}

confirm() {
	if [ "$assume_yes" -eq 1 ]; then
		return 0
	fi
	if [ ! -t 0 ]; then
		die "not a terminal, so nothing was installed; re-run with -y"
	fi

	local reply
	printf '\nProceed? [y/N] '
	read -r reply
	case "$reply" in
	y | Y | yes | Yes) return 0 ;;
	*) return 1 ;;
	esac
}

# ---------------------------------------------------------------- installs

# do_install_piper uses whichever user-scope installer is present. Neither
# writes outside your home directory, and this script installs neither of
# them: bootstrapping a Python tool manager is its own decision.
do_install_piper() {
	if have uv; then
		run uv tool install piper-tts
	elif have pipx; then
		run pipx install piper-tts
	else
		die "neither uv nor pipx found; install one of them first (uv: https://docs.astral.sh/uv, pipx: your distro packages it as pipx)"
	fi
}

# do_install_voice prefers Piper's own downloader, which knows the catalogue
# layout. Without uv there is no way to reach it — a pipx install is sealed
# inside its own venv — so the two files come straight from the repository.
do_install_voice() {
	run mkdir -p "$VOICE_DIR"

	if have uv; then
		run uvx --from piper-tts python -m piper.download_voices "$VOICE" --data-dir "$VOICE_DIR"
		return
	fi

	local file
	for file in "$VOICE.onnx" "$VOICE.onnx.json"; do
		# Downloaded beside the target and moved into place, so an interrupted
		# transfer cannot leave a half file that looks like a working voice.
		run curl -fL --progress-bar -o "$VOICE_DIR/$file.part" "$VOICE_BASE_URL/$file"
		run mv "$VOICE_DIR/$file.part" "$VOICE_DIR/$file"
	done
}

# ------------------------------------------------------------------ report

report() {
	printf '\n'

	if [ "$install_piper" -eq 1 ]; then
		case ":$PATH:" in
		*":$HOME/.local/bin:"*) ;;
		*) printf 'Piper landed in %s/.local/bin, which is not on your PATH.\n' "$HOME" ;;
		esac
	fi

	if [ "${#missing_audio[@]}" -gt 0 ]; then
		printf 'The audio stack is still missing — install it with the command above.\n'
		return 1
	fi

	if have pactl && ! pactl info >/dev/null 2>&1; then
		printf 'pactl is installed but no sound server answers it. Start one with:\n'
		printf '  systemctl --user enable --now pipewire-pulse\n'
		return 1
	fi

	printf 'Done. voitts-server checks all of this at startup and reports what it finds.\n'
}

# -------------------------------------------------------------------- main

main() {
	while getopts ':ynh' opt; do
		case "$opt" in
		y) assume_yes=1 ;;
		n) dry_run=1 ;;
		h)
			usage
			exit 0
			;;
		*)
			usage >&2
			exit 2
			;;
		esac
	done

	[ "$(uname -s)" = Linux ] || die "Linux only: the virtual microphone is built from PipeWire modules"

	detect_family
	collect

	if [ "${#missing_audio[@]}" -eq 0 ] && [ "$install_piper" -eq 0 ] && [ "$install_voice" -eq 0 ]; then
		printf 'Everything voitts-server needs is already installed.\n'
		exit 0
	fi

	describe_audio
	describe_piper

	if [ "$dry_run" -eq 1 ]; then
		exit 0
	fi

	if [ "$install_piper" -eq 1 ] || [ "$install_voice" -eq 1 ]; then
		confirm || die "nothing was installed"
		printf '\n'
		if [ "$install_piper" -eq 1 ]; then
			do_install_piper
		fi
		if [ "$install_voice" -eq 1 ]; then
			do_install_voice
		fi
	fi

	report
}

main "$@"
