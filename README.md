# VoiTTS

**Type on your phone, speak through your PC.** **VoiTTS** turns anything you type into speech on a virtual microphone every app on the machine sees as real.

> This is a Linux-first application, your Windows experience may vary.

## Index
- [Overview](#overview) - the [web interface](#web-interface) and the [audio stack](#audio-stack)
- [Installation](#installation)
    - ["Just give me exe"](#just-give-me-exe-type-installation) - easiest setup: [Linux](#linux), [Windows](#windows)
    - [Runtime prerequisites](#runtime-prerequisites) - needed by all three methods below
        - [1. Audio stack](#1-audio-stack) - PipeWire modules, or VB-CABLE
        - [2. Piper + voice model](#2-piper-tts-engine--voice-model)
    - [Method 1: Compiled binary](#method-1-compiled-binary) - unpack a release and run it
    - [Method 2: Build with the release script](#method-2-build-with-the-release-script) - one script, from a checkout
    - [Method 3: Manual build](#method-3-manual-build) - `pnpm build` then `go build`
    - [Check if it works](#check-if-it-works) - the startup report, and picking the mic
- [AI usage](#ai-usage)

# Overview

## Web interface

Mobile-first web UI, accessed over LAN.
- Send instant TTS messages.
- Invoke pre-written sentences.
- Group texts in custom profiles.

## Audio stack

Uses [Piper](https://github.com/OHF-Voice/piper1-gpl) TTS engine to synthesize speech and routes the audio into a virtual capture device, that other applications see as a real microphone.

How that device is made depends on the platform, and it is the only part that does:

| | Linux | Windows |
| --- | --- | --- |
| Virtual mic | `module-null-sink` + `module-remap-source`, loaded at startup with `pactl` | [VB-CABLE](https://vb-audio.com/Cable/), installed once as a driver |
| Select in apps | `VoiTTS_Microphone` | `CABLE Output (VB-Audio Virtual Cable)` |
| PCM goes in via | `pw-cat`, or `paplay` as a fallback | `waveOut`, called directly - no helper binary |
| Cleanup on exit | unloads the modules it loaded | nothing to unload |

macOS has neither, and is not supported. The equivalent there is BlackHole or Loopback, and nobody has wired it up.

# Installation

## "Just give me exe" type installation

### Linux

1. Install **[uv (a Python package manager)](https://docs.astral.sh/uv/getting-started/installation/)**. 
2. Download the latest `voitts-server_*_linux_amd64.tar.gz` from [Releases](https://github.com/sherpavel/VoiTTS/releases).
3. Open terminal in downloaded location and run:
    ```sh
    tar -xzf voitts-server_*_linux_amd64.tar.gz
    ./voitts-server_*_linux_amd64/install-piper.sh
    ./voitts-server_*_linux_amd64/install.sh
    ```
4. You should see the **VoiTTS** app in your application menu, or a PATH binary `voitts-server`.
5. Run it and if you see a QR code - all good! If not, the app will say what failed.
6. Use `VoiTTS_Microphone` as your input device in other applications.

> `install-piper.sh` installs no system packages. If `pactl` or a PCM player is
missing it prints the install command for your distro and leaves it to you - the
server will not start until you run it. See [Runtime prerequisites](#runtime-prerequisites).

### Windows

1. Install **[VB-CABLE](https://vb-audio.com/Cable/)**.
    1. Download, unzip and run VBCABLE_Setup_x64.exe
    2. **REBOOT.**
2. Install **[uv (a Python package manager)](https://docs.astral.sh/uv/getting-started/installation/)**.
3. Open Powershell and run:
    ```ps
    uv tool install piper-tts
    mkdir $env:LOCALAPPDATA\piper\voices
    uvx --from piper-tts python -m piper.download_voices en_US-hfc_male-medium `
        --data-dir $env:LOCALAPPDATA\piper\voices
    ```
4. Download the latest `voitts-server_*_windows_amd64.zip` from [Releases](https://github.com/sherpavel/VoiTTS/releases) and unzip it anywhere.
5. Run `voitts-server.exe` and if you see a QR code - all good! If not, the app will say what failed.
6. Use `CABLE Output` as your input device in other applications.

## Runtime prerequisites

### 1. Audio stack

#### Linux

> This should come pre-installed on most distros, but check yours.
>
> **VoiTTS** uses `pactl` to load its null sink and remapped source, and `pw-cat` or `paplay` to push PCM into that sink. Both must be in `PATH`.

| Distro | Required | Optional |
| --- | --- | --- |
| Arch / CachyOS | `libpulse` (`pactl`, `paplay`), `pipewire-pulse` | `pipewire-audio` (`pw-cat`) |
| Debian / Ubuntu | `pulseaudio-utils`, `pipewire-pulse` | `pipewire-bin` (`pw-cat`) |
| Fedora | `pulseaudio-utils`, `pipewire-pulseaudio` | `pipewire-utils` (`pw-cat`) |

> `pw-cat` is preferred when present, `paplay` is the fallback. Plain PulseAudio
works too - only `pactl` and one of the two players are actually required.

#### Windows

Install **[VB-CABLE](https://vb-audio.com/Cable/)**. It is an audio driver, and it is the whole audio stack.

1. Download the zip, right-click `VBCABLE_Setup_x64.exe` and **Run as administrator**.
2. Reboot.

You should then see a matched pair in the Windows sound settings - `CABLE Input` under playback, `CABLE Output` under recording. VoiTTS writes into the first; other applications listen to the second.

> The A/B and Hi-Fi Cable variants of the same driver work too, if you already have one installed. The server takes the first it finds, preferring plain `CABLE Input`.

### 2. Piper (TTS engine) + voice model

There are 2 ways to install:

1. Use the bundled `install-piper.sh` script. **Linux only.**
2. Install via `uv`/`pipx` and download the voice model.

#### Using `install-piper.sh`

Run `install-piper.sh` script. It comes bundled in the `.tar.gz` from the Releases, or download it from [`assets/bundle/install-piper.sh`](assets/bundle/install-piper.sh).
It checks the same three things the server checks at startup - the audio stack, Piper and the voice - then installs Piper and the voice into your home directory with `uv` or `pipx`, and nothing else. System packages are never installed: a missing `pactl` or PCM player is reported with the command for your distro, and left to you.

#### Manual + voice model
Install the Python package, not a distro package named `piper`:

```sh
uv tool install piper-tts     # or: pipx install piper-tts
```

> The server looks for Piper in this order: the Python module, then a `piper`
executable on `PATH`, then the directories `uv` and `pipx` install console
scripts into - `$UV_TOOL_BIN_DIR`, `$PIPX_BIN_DIR`, and `~/.local/bin`.
>
> That last step matters on Windows, where `uv tool install piper-tts` succeeds
and leaves nothing on `PATH`: `%USERPROFILE%\.local\bin` is not on it by
default and nothing puts it there. The server finds Piper anyway. If you would
rather fix it globally - which also helps every other tool `uv` installs - run
`uv tool update-shell` and restart your shell.
>
> Which interpreters it tries differs too: `python3` then `python` on Linux,
`python` then the `py` launcher on Windows, where `python3` is usually the
Microsoft Store stub rather than an interpreter.

Voices are not bundled with Piper. Download the default one (`en_US-hfc_male-medium`)
into a directory the server searches:

```sh
# Linux
mkdir -p ~/.local/share/piper/voices
uvx --from piper-tts python -m piper.download_voices en_US-hfc_male-medium \
  --data-dir ~/.local/share/piper/voices
```

```powershell
# Windows
mkdir $env:LOCALAPPDATA\piper\voices
uvx --from piper-tts python -m piper.download_voices en_US-hfc_male-medium `
  --data-dir $env:LOCALAPPDATA\piper\voices
```

That writes `en_US-hfc_male-medium.onnx` and its manifest `.onnx.json`, which
holds the sample rate - both are required.

> Windows searches `%LOCALAPPDATA%\piper\voices` first, then `%APPDATA%`, then
the Linux-style `~/.local/share/piper/voices`, then the working directory and
the folder the `.exe` sits in. LocalAppData leads because a voice is tens of
megabytes and Roaming profiles copy their contents between machines at logon.
Whatever the server ends up searching, it prints the full list when it cannot
find a voice.

## Method 1: Compiled binary

Download the latest archive for your platform from
[Releases](https://github.com/sherpavel/VoiTTS/releases):

```sh
# Linux
tar -xzf voitts-server_*_linux_amd64.tar.gz
./voitts-server_*_linux_amd64/install.sh
```

The install script puts the binary in `~/.local/bin` and generates the `.desktop` file

```powershell
# Windows
Expand-Archive voitts-server_*_windows_amd64.zip -DestinationPath voitts-server
.\voitts-server\voitts-server.exe
```

The zip is flat - `Expand-Archive` unpacks into whatever `-DestinationPath` names, and Explorer's **Extract All** makes a folder of its own. There is no install script on Windows, because there is nothing to install: unzip it anywhere and run the `.exe`. A shortcut is yours to make, and takes its icon from the binary.

## Method 2: Build with the release script

Needs **Go 1.27+**, **Node.js** `^20.19` or `>=22.12`, and **pnpm 9+**.

```sh
# Linux
git clone https://github.com/sherpavel/VoiTTS.git
cd VoiTTS
./tools/build-release-linux.sh v0.1.0

tar -xzf out/voitts-server_v0.1.0_linux_amd64.tar.gz -C out
./out/voitts-server_v0.1.0_linux_amd64/install.sh
```

```powershell
# Windows
git clone https://github.com/sherpavel/VoiTTS.git
cd VoiTTS
.\tools\build-release-windows.ps1 -Version v0.1.0

Expand-Archive out\voitts-server_v0.1.0_windows_amd64.zip -DestinationPath out\voitts-server
.\out\voitts-server\voitts-server.exe
```

Each script builds the web UI, then a binary stamped with the version you
passed - `voitts-server -version` prints it back. On Windows that version also
goes into the exe's resources, alongside the icon, so it shows up in the
**Details** tab of the file's properties without the file being run. They
cross-compile nothing: run the one for the platform you are on.

On Linux this then uses the same install script as in [compiled binary installation](#method-1-compiled-binary).

## Method 3: Manual build

The web UI is compiled into the binary with `//go:embed`, so **build the
frontend first**. Skipping it still compiles, but the server then refuses to
start:

```
create webui handler: web: no index.html in the embedded build; run `pnpm build` in webui/
```

```sh
# Linux
git clone https://github.com/sherpavel/VoiTTS.git
cd VoiTTS

pnpm --dir webui install && pnpm --dir webui build   # writes internal/web/dist/app
CGO_ENABLED=0 go build -ldflags="-s -w" -o out/voitts-server ./cmd/server

./assets/bundle/install.sh
```

```powershell
# Windows
git clone https://github.com/sherpavel/VoiTTS.git
cd VoiTTS

pnpm --dir webui install; pnpm --dir webui build   # writes internal/web/dist/app
$env:CGO_ENABLED = '0'
go build -ldflags "-s -w" -o out\voitts-server.exe .\cmd\server
```

There is no install script on Windows - the `.exe` runs from wherever you put it.

On Linux `CGO_ENABLED=0` is what makes it static; without it the binary links
against the host's libc through cgo's resolver and may not run elsewhere. On
Windows it changes nothing that matters - there is no libc to be pinned to, and
the audio path is plain syscalls into `winmm.dll` - but the release script sets
it there too, so that a machine with a C toolchain installed builds the same
binary as one without.

For frontend work, `pnpm --dir webui dev` proxies `/api` to a server already running on
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

The first two lines are what differs on Windows, where the audio stack is one
driver rather than two binaries:

```
> voitts-server.exe

  + vb-cable        CABLE Input (VB-Audio Virtual C
  + capture device  CABLE Output (VB-Audio Virtual
  + piper           C:\Users\you\.local\bin\piper.exe
  + voice           C:\Users\you\AppData\Local\piper\voices\en_US-hfc_male-medium.onnx
                    (22050 Hz)
```

> The device names really are cut off like that. Windows reports them through a
32-character field, and the report prints what it was given rather than
inventing the rest.
>
> Paths are printed in full on Windows rather than shortened to `~`, because
`~` is not Windows syntax. PowerShell expands it for its own cmdlets, but hands
it to a native program verbatim - so a `--data-dir ~\...` copied out of a fix
line would put the voice in a directory literally named `~`. The Linux report
still shortens, where every shell expands it.

Whatever is missing is marked `x` and followed by the command that installs it,
and the server exits instead of starting half-equipped.

It then prints the URL to open, with a QR code for it, and the capture device
appears:

> On Linux the URL is the machine's `.local` name where Avahi publishes one,
with the IP address offered beside it. Windows is given the address only.
Windows will answer an mDNS query for its own name, but whether one from the
network reaches it at all depends on the firewall profile - and a lookup made
on the machine itself cannot tell, because that one arrives over loopback
where the inbound rules do not apply. A name that resolves only on the machine
printing it is worse than no name, so the address goes in the QR code.

```sh
# Linux
pactl list short sources | grep voitts_mic
```

On Windows it is `CABLE Output`, under **Recording** in the sound settings.

Pick **VoiTTS_Microphone** (Linux) or **CABLE Output** (Windows) as the input
device in whatever app should hear it. The audio is mirrored to your own
speakers as it is sent, so you hear what the other side hears.

The server listens on port **17890**, on **all interfaces** — anyone who can
reach the machine can make its microphone speak, so keep it off untrusted
networks. If something already holds the port it refuses to start rather than
moving somewhere else, so the address stays the one your phone has:

```sh
voitts-server          # then open http://localhost:17890
```

On Linux it unloads the PipeWire modules it created on `SIGINT`, `SIGTERM` and
`SIGHUP`, so exit it properly rather than with `SIGKILL` — otherwise a stray
sink and source are left behind on every run. Windows has no such tidying to
do: VB-CABLE is a driver, the server only borrows it, and `Ctrl-C` or closing
the window leaves nothing behind either way.

---

# AI usage

Parts of this project were written with AI. Everything that ships was read, edited and tested. Nothing here was committed unseen.

**Scripts.** Everything in [`tools/`](tools/) and [`assets/bundle/`](assets/bundle/) is AI-written in full - the shell scripts and the PowerShell ones alike - and each carries the same warning in the header.

**Go.** Internal Piper and audio backends, and the pre-run check: [`internal/tts`](internal/tts/tts.go), [`internal/audio`](internal/audio/), [`cmd/server/check.go`](cmd/server/check.go) and its two platform halves. The winmm bindings in [`internal/audio/winmm_windows.go`](internal/audio/winmm_windows.go) especially - hand-written syscall shims are exactly the sort of thing worth a second read.

**Svelte.** The drag-to-reorder action (`use:`): [`sortable.svelte.ts`](webui/src/lib/actions/sortable.svelte.ts).

**Docs.** Some of this README.
