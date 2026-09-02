#Requires -Version 5.1
#
# THIS IS AI GENERATED, PROCEED WITH CAUTION
#
# Builds a voitts-server release zip for windows/amd64 into out/, ready to
# upload to a GitHub release by hand. The Windows counterpart of
# build-release.sh, which does the same for linux/amd64.
#
# The web UI is compiled into the binary by internal/web, so the frontend is
# built first. That order matters: `go build` succeeds against an empty
# internal/web/dist/app and produces a server that then refuses to start, which
# is what the index.html check below is guarding against.
#
# CGO is off, as it is on Linux. It buys less here -- there is no libc to be
# pinned to -- but it keeps the build reproducible on a machine that happens to
# have a C toolchain installed, and the audio path is pure syscall either way.
#
# Unlike the Linux tarball there is no install script beside the binary: there
# is nothing to install. Unpack it and run voitts-server.exe. What the release
# cannot bring with it is VB-CABLE, which is a signed driver from
# https://vb-audio.com/Cable/ and has to be installed by hand; the server's
# startup check says so if it is missing.

[CmdletBinding()]
param(
	[Parameter(Mandatory = $true, HelpMessage = 'Release version, e.g. v0.1.0')]
	[ValidateNotNullOrEmpty()]
	[string]$Version
)

# The cmdlet half of `set -e`. Native programs ignore it, so every exe below is
# followed by an explicit exit-code check.
$ErrorActionPreference = 'Stop'

function Invoke-Checked {
	param([string]$What, [scriptblock]$Command)

	& $Command
	if ($LASTEXITCODE -ne 0) {
		throw "$What failed with exit code $LASTEXITCODE"
	}
}

# Run from the repo root whatever directory this was invoked from.
Push-Location (Join-Path $PSScriptRoot '..')
try {
	Invoke-Checked 'pnpm install' { pnpm --dir webui install --frozen-lockfile }
	Invoke-Checked 'pnpm build' { pnpm --dir webui build }

	$index = 'internal/web/dist/app/index.html'
	if (-not (Test-Path $index) -or (Get-Item $index).Length -eq 0) {
		throw 'build-release: pnpm build left nothing for internal/web to embed'
	}

	# Everything is staged under one directory so the archive unpacks into a
	# folder of its own. A flat zip spills its files over whatever directory it
	# is opened in, overwriting ones that happen to share those names.
	$name = "voitts-server_${Version}_windows_amd64"
	$stage = "out/$name"
	if (Test-Path $stage) { Remove-Item $stage -Recurse -Force }
	New-Item -ItemType Directory -Path $stage -Force | Out-Null

	# Saved and put back rather than set for the session: dot-sourcing this
	# script would otherwise leave the caller's shell cross-compiling.
	$saved = @{
		CGO_ENABLED = $env:CGO_ENABLED
		GOOS        = $env:GOOS
		GOARCH      = $env:GOARCH
	}
	try {
		$env:CGO_ENABLED = '0'
		$env:GOOS = 'windows'
		$env:GOARCH = 'amd64'
		Invoke-Checked 'go build' {
			go build -ldflags "-s -w -X main.version=$Version" -o "$stage/voitts-server.exe" ./cmd/server
		}
	} finally {
		foreach ($key in $saved.Keys) {
			# Unset, rather than set to empty: an exported GOOS="" is not the
			# same to the Go toolchain as no GOOS at all.
			if ($null -eq $saved[$key]) {
				Remove-Item "env:$key" -ErrorAction SilentlyContinue
			} else {
				Set-Item -Path "env:$key" -Value $saved[$key]
			}
		}
	}

	Copy-Item README.md, LICENSE -Destination $stage

	# The icon, for anyone making a Start Menu or desktop shortcut by hand.
	# Only the .ico: the web icons live in webui/static and are already inside
	# the binary.
	New-Item -ItemType Directory -Path "$stage/icons" -Force | Out-Null
	Copy-Item assets/icons/favicon.ico -Destination "$stage/icons/voitts.ico"

	Compress-Archive -Path $stage -DestinationPath "out/$name.zip" -Force
	Remove-Item $stage -Recurse -Force

	Get-ChildItem out | Select-Object Name, @{
		Name       = 'Size'
		Expression = { '{0:N1} MB' -f ($_.Length / 1MB) }
	}
} finally {
	Pop-Location
}
