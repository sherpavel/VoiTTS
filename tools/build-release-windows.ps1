#Requires -Version 5.1
#
# THIS IS AI GENERATED, PROCEED WITH CAUTION
#
# Builds a voitts-server release zip for windows/amd64 into out/, ready to
# upload to a GitHub release by hand. The Windows counterpart of
# build-release-linux.sh, which does the same for linux/amd64.
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
# The exe carries its own icon and its own version, which on Windows means
# compiled resources rather than a file beside it or an -ldflags string.
# win-resources.ps1 builds the object the Go linker folds in; see the top of
# that script for how. -X main.version is still set as well: that is the one
# the running program prints, and the resource is the one Explorer reads.
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

# Assert-Resources is here because the icon and the version are the parts of
# this build that fail quietly: a resource object the linker did not care for
# leaves an exe that runs perfectly and is merely blank in Explorer, which is
# not something the next person to open the zip should be first to discover.
function Assert-Resources {
	param([string]$Path, [string]$Version)

	$exe = (Resolve-Path -LiteralPath $Path).ProviderPath

	# Windows reads the version resource itself, so this needs nothing but a
	# file. An exe without one answers with a null rather than an error.
	$found = (Get-Item -LiteralPath $exe).VersionInfo.FileVersion
	if ($found -ne $Version) {
		throw "build-release: the binary reports version '$found', not '$Version'"
	}

	# Nothing reads the icon out for free, though. Asking PrivateExtractIcons
	# for no icons is how it is asked to count the ones in a file.
	$signature = @'
[DllImport("user32.dll", CharSet = CharSet.Unicode)]
public static extern int PrivateExtractIconsW(string file, int index, int cx,
	int cy, IntPtr[] icons, int[] ids, int count, int flags);
'@
	try {
		$user32 = Add-Type -MemberDefinition $signature -Name 'IconProbe' `
			-Namespace 'VoittsBuild' -PassThru -ErrorAction Stop
	} catch {
		# A machine without a C# compiler can still cut a release; it just
		# does so without this half of the check.
		Write-Warning "build-release: cannot check the icon: $_"
		return
	}

	$icons = $user32::PrivateExtractIconsW($exe, 0, 0, 0, $null, $null, 0, 0)
	if ($icons -lt 1) {
		throw 'build-release: the binary came out with no icon'
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

	# A staging directory only so the zip is built from a known set of files.
	# It is not in the archive: unlike the Linux tarball, which needs a folder
	# of its own or `tar -x` scatters the release over the current directory,
	# Explorer's Extract All already unpacks into a folder named after the zip.
	# Shipping one as well would only nest it inside that.
	$name = "voitts-server_${Version}_windows_amd64"
	$stage = "out/$name"
	if (Test-Path $stage) { Remove-Item $stage -Recurse -Force }
	New-Item -ItemType Directory -Path $stage -Force | Out-Null

	# The icon and the version have to be beside the sources for `go build` to
	# notice them: the linker picks up any *.syso in the package directory, and
	# the _windows_ and _amd64 in the name are what keep it out of a Linux
	# build. It is a build artifact rather than a source file, so it is
	# compiled fresh here and taken away below -- left behind, it would go on
	# stamping every later `go build` with this release's version.
	$syso = 'cmd/server/rsrc_windows_amd64.syso'
	& (Join-Path $PSScriptRoot 'win-resources.ps1') -IconPath 'assets/icons/favicon.ico' `
		-OutPath $syso -Version $Version

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
		Remove-Item $syso -Force -ErrorAction SilentlyContinue

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

	Assert-Resources "$stage/voitts-server.exe" $Version

	Copy-Item README.md, LICENSE -Destination $stage

	# The trailing \* is what keeps the zip flat: pointed at the directory
	# itself, Compress-Archive puts that directory *inside* the archive, and
	# Explorer's Extract All -- which already makes a folder of its own, named
	# after the zip -- would then nest one inside the other.
	Compress-Archive -Path "$stage/*" -DestinationPath "out/$name.zip" -Force
	Remove-Item $stage -Recurse -Force

	Get-ChildItem out | Select-Object Name, @{
		Name       = 'Size'
		Expression = { '{0:N1} MB' -f ($_.Length / 1MB) }
	}
} finally {
	Pop-Location
}
