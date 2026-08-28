# VoiTTS

**Type on your phone, speak through your PC.** VoiTTS turns anything you type into speech on a virtual microphone every app on the machine sees as real.

## Index
- [About](#about)
- [Installation](#installation)
    - [Prerequisites](#runtime-prerequisites) - REQUIRED for all methods
        - [Pre-compiled from release](#method-1-compiled-binary-recommended) (Recommended)
        - [Using `build` script](#method-2-build-with-the-release-script)
        - [Manually](#method-3-manual-build)

# Overview

## Web interface

Mobile-first web UI, accessed over LAN.
- Send instant TTS messages.
- Invoke pre-written sentences.
- Group texts in custom profiles.

## Audio stack

Uses [Piper](https://github.com/OHF-Voice/piper1-gpl) TTS engine to synthesize speech and routes the audio into a virtual PipeWire/PulseAudio capture device, that other applications see as a real microphone.

**Linux only** - the virtual microphone is built from PipeWire/PulseAudio modules.

## Is there a Windows version?
**No**. If someone wants to fork, godspeed. Windows Core Audio sure is something.

# Installation

## Runtime prerequisites

### 1. Audio stack

> This should come pre-installed on most distros, but check your's.

VoiTTS uses `pactl` to load its null sink and remapped source, and `pw-cat` or `paplay` to push PCM into that sink. Both must be in `PATH`.

| Distro | Required | Optional |
| --- | --- | --- |
| Arch / CachyOS | `libpulse` (`pactl`, `paplay`), `pipewire-pulse` | `pipewire-audio` (`pw-cat`) |
| Debian / Ubuntu | `pulseaudio-utils`, `pipewire-pulse` | `pipewire-bin` (`pw-cat`) |
| Fedora | `pulseaudio-utils`, `pipewire-pulseaudio` | `pipewire-utils` (`pw-cat`) |

> `pw-cat` is preferred when present, `paplay` is the fallback. Plain PulseAudio
works too - only `pactl` and one of the two players are actually required.

### 2. Piper (TTS engine) + voice model

There are 2 ways to install:

1. Use the bundled `install-piper.sh` script.
2. Install via `uv`/`pipx` and download the voice model.

#### Using `install-piper.sh`

Run `install-piper.sh` script. It comes bundled in `.tar` from the Releases or download it from `scripts/install-piper.sh`.
It checks for missing Python packages, then installs Piper and the voice into your home directory with `uv` or `pipx` - and nothing else.

#### Manual + voice model
Install the Python package, not a distro package named `piper`:

```sh
uv tool install piper-tts     # or: pipx install piper-tts
```

> The server looks for Piper in this order: `python3 -m piper`, then `python -m piper`, then a `piper` executable on `PATH`.

Voices are not bundled with Piper. Download the default one (`en_US-hfc_male-medium`)
into a directory the server searches:

```sh
mkdir -p ~/.local/share/piper/voices
uvx --from piper-tts python -m piper.download_voices en_US-hfc_male-medium \
  --data-dir ~/.local/share/piper/voices
```

That writes `en_US-hfc_male-medium.onnx` and its manifest `.onnx.json`, which
holds the sample rate - both are required.

## Method 1: Compiled binary (Recommended)

Download the latest `linux/amd64` tarball from
[Releases](https://github.com/sherpavel/VoiTTS/releases):

```sh
tar -xzf voitts-server_*_linux_amd64.tar.gz
./voitts-server_*_linux_amd64/install.sh
```

The install script puts the binary in `~/.local/bin` and generates the `.desktop` file

## Method 2: Build with the release script

Needs **Go 1.27+**, **Node.js** `^20.19` or `>=22.12`, and **pnpm 9+**.

```sh
git clone https://github.com/sherpavel/VoiTTS.git
cd VoiTTS
./scripts/build-release.sh v0.1.0

tar -xzf out/voitts-server_v0.1.0_linux_amd64.tar.gz -C out
./out/voitts-server_v0.1.0_linux_amd64/install.sh
```

The script builds the web UI, then a static binary stamped with the version
you passed - `voitts-server -version` prints it back.

Then uses the same install script as in [compiled binary installation](#method-1-compiled-binary-recommended).

## Method 3: Manual build

The web UI is compiled into the binary with `//go:embed`, so **build the
frontend first**. Skipping it still compiles, but the server then refuses to
start:

```
create webui handler: web: no index.html in the embedded build; run `pnpm build` in webui/
```

```sh
git clone https://github.com/sherpavel/VoiTTS.git
cd VoiTTS

pnpm --dir webui install && pnpm --dir webui build   # writes internal/web/dist/app
CGO_ENABLED=0 go build -ldflags="-s -w" -o out/voitts-server ./cmd/server

./scripts/install.sh
```

`CGO_ENABLED=0` is what makes it static; without it the binary links against
the host's libc through cgo's resolver and may not run elsewhere.

For frontend work, `pnpm dev` proxies `/api` to a server already running on
17890, so keep `voitts-server` up in another shell.

## Check if it works

Start it. Before anything else it reports the four things it needs:

```
$ voitts-server

  + audio server  PulseAudio (on PipeWire 1.6.8) (/usr/bin/pactl)
  + pcm player    /usr/bin/pw-cat
  + piper         ~/.local/bin/piper
  + voice         ~/.local/share/piper/voices/en_US-hfc_male-medium.onnx (22050 Hz)
```

Whatever is missing is marked `x` and followed by the command that installs it,
and the server exits instead of starting half-equipped.

It then prints the URL to open, with a QR code for it, and the capture device
appears:

```sh
pactl list short sources | grep voitts_mic
```

Pick **VoiTTS_Microphone** as the input device in whatever app should hear it.
The audio is mirrored to your own speakers as it is sent, so you hear what the
other side hears.

The server listens on port **17890**, on **all interfaces** — anyone who can
reach the machine can make its microphone speak, so keep it off untrusted
networks. If something already holds the port it refuses to start rather than
moving somewhere else, so the address stays the one your phone has:

```sh
voitts-server          # then open http://localhost:17890
```

It unloads the PipeWire modules it created on `SIGINT`, `SIGTERM` and `SIGHUP`,
so exit it properly rather than with `SIGKILL` — otherwise a stray sink and
source are left behind on every run.

---

# AI usage

Parts of this project were written with AI. Everything that ships was read, edited and tested. Nothing here was committed unseen.

**Shell scripts.** All scripts in [`scripts/`](scripts/) are AI-written in full, and each carries the same warning in the header. 

**Go.** Internal Piper and PipeWire modules, and pre-run check: [`internal/tts`](internal/tts/tts.go), [`internal/audio`](internal/audio/audio.go), [`cmd/server/check.go`](cmd/server/check.go).

**Svelte.** The drag-to-reorder action (`use:`): [`sortable.svelte.ts`](webui/src/lib/actions/sortable.svelte.ts).

**Docs.** Some of this README.
