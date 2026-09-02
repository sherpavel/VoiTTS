#Requires -Version 5.1
#
# THIS IS AI GENERATED, PROCEED WITH CAUTION
#
# Compiles the two things a Windows exe cannot carry as plain data -- its icon
# and its version -- into the COFF object the Go linker folds into the binary.
# build-release-windows.ps1 runs this just before it builds.
#
# The icon is the one Explorer, the taskbar and Alt-Tab draw, and the one a
# shortcut picks up without being pointed at a file. The version is what the
# Details tab of the file's properties reads, and with it every tool that goes
# looking there: installers, software inventories, `(Get-Item x).VersionInfo`.
# Note that `-ldflags -X main.version` does *not* reach any of them -- that
# sets a Go string, which only the running program can see.
#
# Go has no resource compiler, but its linker does pick up any *.syso sitting
# beside a package's sources and folds that object's sections into the image.
# So the resources have to arrive already compiled, which is what this writes
# -- rather than fetching goversioninfo or rsrc over the network in the middle
# of a release build.
#
# What goes into the object is a .rsrc section: three levels of
# IMAGE_RESOURCE_DIRECTORY (type, then name, then language) over one
# IMAGE_RESOURCE_DATA_ENTRY per resource, holding the three types Windows
# looks for:
#
#   RT_ICON (3)         one per image in the .ico, ids 1..N, each holding that
#                       image exactly as the .ico stored it
#   RT_GROUP_ICON (14)  id 1, the directory Windows reads first to choose
#                       which of those images to draw at the size it needs
#   RT_VERSION (16)     id 1, the numbers and the strings, present only when
#                       -Version is passed
#
# The part that makes this more than a byte dump is that each DATA_ENTRY finds
# its bytes by RVA -- an address in the linked image, which nothing here can
# know. Every entry therefore carries a relocation against the section symbol
# and the Go linker fills the address in. Without them the icon still links,
# pointing at whatever happens to sit at the start of the image.
#
# amd64 only, which is all build-release-windows.ps1 ships. The one thing that
# would have to change for 386 is the relocation type.

[CmdletBinding()]
param(
	[Parameter(Mandatory = $true, HelpMessage = 'The .ico to compile')]
	[ValidateNotNullOrEmpty()]
	[string]$IconPath,

	[Parameter(Mandatory = $true, HelpMessage = 'The .syso to write')]
	[ValidateNotNullOrEmpty()]
	[string]$OutPath,

	[Parameter(HelpMessage = 'Release version, e.g. v0.1.0. Omitted, no version resource is written')]
	[string]$Version
)

$ErrorActionPreference = 'Stop'

$RT_ICON = 3
$RT_GROUP_ICON = 14
$RT_VERSION = 16

# The rest of what the Details tab shows. None of it varies per release, and
# the copyright is the line from LICENSE.
$COMPANY_NAME = 'Pavel Sherstyuk'
$PRODUCT_NAME = 'VoiTTS'
$FILE_DESCRIPTION = 'VoiTTS server'
$INTERNAL_NAME = 'voitts-server'
$ORIGINAL_FILENAME = 'voitts-server.exe'
$LEGAL_COPYRIGHT = 'Copyright (c) 2026 Pavel Sherstyuk'

# en-US, which is what rc.exe stamps on a resource by default. Windows falls
# back across languages when it looks an icon up, so this is a label on the
# resource rather than a restriction on who sees it.
$LANG_EN_US = 0x0409

function Read-U16 { param([byte[]]$Bytes, [int]$At) [BitConverter]::ToUInt16($Bytes, $At) }
function Read-U32 { param([byte[]]$Bytes, [int]$At) [BitConverter]::ToUInt32($Bytes, $At) }

function Resolve-Full {
	param([string]$Path)

	if (-not [System.IO.Path]::IsPathRooted($Path)) {
		$Path = Join-Path (Get-Location).ProviderPath $Path
	}
	[System.IO.Path]::GetFullPath($Path)
}

# ---------------------------------------------------------------- the .ico

$ico = [System.IO.File]::ReadAllBytes((Resolve-Full $IconPath))

# ICONDIR: two reserved bytes, type 1 for an icon (2 is a cursor), image count.
if ($ico.Length -lt 6 -or (Read-U16 $ico 0) -ne 0 -or (Read-U16 $ico 2) -ne 1) {
	throw "win-resources: $IconPath is not a Windows .ico"
}
$count = Read-U16 $ico 4
if ($count -eq 0) { throw "win-resources: $IconPath holds no images" }
if ($ico.Length -lt 6 + 16 * $count) { throw "win-resources: $IconPath is truncated" }

$images = @()
for ($i = 0; $i -lt $count; $i++) {
	# ICONDIRENTRY: width, height, colours, reserved, planes, bit depth, then
	# the length and file offset of the image itself.
	$at = 6 + 16 * $i
	$size = Read-U32 $ico ($at + 8)
	$offset = Read-U32 $ico ($at + 12)
	if ($size -eq 0 -or [int64]$offset + $size -gt $ico.Length) {
		throw "win-resources: image $($i + 1) of $IconPath runs past the end of the file"
	}

	$data = New-Object byte[] $size
	[Array]::Copy($ico, $offset, $data, 0, $size)

	# Plenty of icon writers leave planes and bit depth zero in the directory.
	# Windows picks which image to draw out of the group directory below, so
	# those zeros would be what it chooses on. Both numbers are in the image:
	# a PNG entry is always 32-bit, a BMP one says so in its BITMAPINFOHEADER.
	$planes = Read-U16 $ico ($at + 4)
	$bits = Read-U16 $ico ($at + 6)
	if ($planes -eq 0 -or $bits -eq 0) {
		if ($data.Length -ge 8 -and $data[0] -eq 0x89 -and $data[1] -eq 0x50) {
			$planes = 1
			$bits = 32
		} elseif ($data.Length -ge 16) {
			$planes = Read-U16 $data 12
			$bits = Read-U16 $data 14
		}
	}

	$images += [pscustomobject]@{
		# Zero means 256 in a .ico, and is stored that way in the group
		# directory too, so both bytes are passed through untouched.
		Width  = $ico[$at]
		Height = $ico[$at + 1]
		Colors = $ico[$at + 2]
		Planes = $planes
		Bits   = $bits
		Data   = $data
	}
}

# GRPICONDIR: the same header the .ico has, followed by 14-byte entries that
# swap the file offset for the id of the RT_ICON holding those bytes.
$group = New-Object System.IO.MemoryStream
$groupWriter = New-Object System.IO.BinaryWriter($group)
$groupWriter.Write([uint16]0)
$groupWriter.Write([uint16]1)
$groupWriter.Write([uint16]$count)
for ($i = 0; $i -lt $count; $i++) {
	$image = $images[$i]
	$groupWriter.Write([byte]$image.Width)
	$groupWriter.Write([byte]$image.Height)
	$groupWriter.Write([byte]$image.Colors)
	$groupWriter.Write([byte]0)
	$groupWriter.Write([uint16]$image.Planes)
	$groupWriter.Write([uint16]$image.Bits)
	$groupWriter.Write([uint32]$image.Data.Length)
	$groupWriter.Write([uint16]($i + 1))
}
$groupWriter.Flush()

# ---------------------------------------------------------------- the version

# A VS_VERSIONINFO is a tree of one node shape, nested three deep: a length, a
# value length, a flag saying whether the value is text or binary, a
# null-terminated UTF-16 key, then the value and any children. Every node
# starts on a 4-byte boundary and every value is padded to one, which is why
# the lengths cannot be worked out in advance -- each is patched in once its
# children have been written.
#
# The tree Windows expects is:
#
#   VS_VERSION_INFO         the four numbers, as a VS_FIXEDFILEINFO
#     StringFileInfo
#       040904B0            en-US in codepage 1200, i.e. UTF-16
#         FileVersion       ... and the rest of the Details tab
#     VarFileInfo
#       Translation         the same pair again, as numbers

function Write-Pad4 {
	param([System.IO.BinaryWriter]$Writer)

	$Writer.Flush()
	while ($Writer.BaseStream.Position % 4 -ne 0) { $Writer.Write([byte]0) }
}

# Write-Node writes a node's header and returns where its length field went,
# for Set-NodeLength to fill in once the node is complete. ValueLength counts
# bytes for a binary value and characters for a text one, the null terminator
# included -- a distinction the format makes and nothing warns about.
function Write-Node {
	param([System.IO.BinaryWriter]$Writer, [string]$Key, [int]$ValueLength, [int]$Type)

	$Writer.Flush()
	$at = $Writer.BaseStream.Position
	$Writer.Write([uint16]0) # patched by Set-NodeLength
	$Writer.Write([uint16]$ValueLength)
	$Writer.Write([uint16]$Type)
	$Writer.Write([System.Text.Encoding]::Unicode.GetBytes($Key))
	$Writer.Write([uint16]0) # the key's null terminator
	Write-Pad4 $Writer
	$at
}

function Set-NodeLength {
	param([System.IO.BinaryWriter]$Writer, [int64]$At)

	$Writer.Flush()
	$end = $Writer.BaseStream.Position
	$Writer.BaseStream.Position = $At
	$Writer.Write([uint16]($end - $At))
	$Writer.Flush()
	$Writer.BaseStream.Position = $end
}

function Write-StringValue {
	param([System.IO.BinaryWriter]$Writer, [string]$Key, [string]$Value)

	Write-Pad4 $Writer
	# The value is counted in characters here, and the terminator counts.
	$at = Write-Node $Writer $Key ($Value.Length + 1) 1
	$Writer.Write([System.Text.Encoding]::Unicode.GetBytes($Value))
	$Writer.Write([uint16]0)
	Set-NodeLength $Writer $at
}

# ConvertTo-FileVersion takes "v0.1.0-rc1" down to the four 16-bit numbers
# VS_FIXEDFILEINFO has room for, dropping the leading v and stopping at the
# first thing that is not a digit. The full string still goes in verbatim as
# FileVersion, which is the field Explorer actually shows.
function ConvertTo-FileVersion {
	param([string]$Version)

	$head = (($Version -replace '^[vV]', '') -split '[^0-9.]', 2)[0]
	$numbers = @($head -split '\.' | Where-Object { $_ -ne '' } | ForEach-Object {
			[Math]::Min([int64]$_, 65535)
		})
	while ($numbers.Count -lt 4) { $numbers += 0 }
	@($numbers[0..3])
}

function New-VersionInfo {
	param([string]$Version)

	$parts = ConvertTo-FileVersion $Version
	$most = [uint32](([int64]$parts[0] * 65536) + $parts[1])
	$least = [uint32](([int64]$parts[2] * 65536) + $parts[3])

	$stream = New-Object System.IO.MemoryStream
	$writer = New-Object System.IO.BinaryWriter($stream)

	$root = Write-Node $writer 'VS_VERSION_INFO' 52 0

	# VS_FIXEDFILEINFO, all 52 bytes of it.
	$writer.Write([uint32]0xFEEF04BDL) # dwSignature
	$writer.Write([uint32]0x00010000)  # dwStrucVersion: 1.0
	$writer.Write($most)               # dwFileVersionMS
	$writer.Write($least)              # dwFileVersionLS
	$writer.Write($most)               # dwProductVersionMS
	$writer.Write($least)              # dwProductVersionLS
	$writer.Write([uint32]0x3F)        # dwFileFlagsMask: every flag is meaningful
	$writer.Write([uint32]0)           # dwFileFlags: no debug, prerelease or patched build
	$writer.Write([uint32]0x00040004)  # dwFileOS: VOS_NT_WINDOWS32
	$writer.Write([uint32]1)           # dwFileType: VFT_APP
	$writer.Write([uint32]0)           # dwFileSubtype: only drivers and fonts use this
	$writer.Write([uint32]0)           # dwFileDateMS
	$writer.Write([uint32]0)           # dwFileDateLS

	Write-Pad4 $writer
	$stringFileInfo = Write-Node $writer 'StringFileInfo' 0 1
	$stringTable = Write-Node $writer '040904B0' 0 1
	Write-StringValue $writer 'CompanyName' $COMPANY_NAME
	Write-StringValue $writer 'FileDescription' $FILE_DESCRIPTION
	Write-StringValue $writer 'FileVersion' $Version
	Write-StringValue $writer 'InternalName' $INTERNAL_NAME
	Write-StringValue $writer 'LegalCopyright' $LEGAL_COPYRIGHT
	Write-StringValue $writer 'OriginalFilename' $ORIGINAL_FILENAME
	Write-StringValue $writer 'ProductName' $PRODUCT_NAME
	Write-StringValue $writer 'ProductVersion' $Version
	Set-NodeLength $writer $stringTable
	Set-NodeLength $writer $stringFileInfo

	# The same language and codepage over again, as a pair of numbers rather
	# than as the name of a string table. Leave it out and some readers decide
	# the strings above are for nobody.
	Write-Pad4 $writer
	$varFileInfo = Write-Node $writer 'VarFileInfo' 0 1
	$translation = Write-Node $writer 'Translation' 4 0
	$writer.Write([uint16]$LANG_EN_US)
	$writer.Write([uint16]0x04B0) # codepage 1200, UTF-16
	Set-NodeLength $writer $translation
	Set-NodeLength $writer $varFileInfo

	Set-NodeLength $writer $root
	$writer.Flush()

	# The comma is load-bearing. PowerShell unrolls an array returned from a
	# function into its output stream, and the caller collects the pieces back
	# into an Object[] of boxed bytes -- which BinaryWriter then does not
	# recognise as bytes at all. Wrapping it makes the array the single item
	# returned, and the caller unwraps it as the byte[] it is.
	, $stream.ToArray()
}

# ---------------------------------------------------------------- resources

# Sorted by type and then by id, because that is how the resource loader reads
# a directory: it binary-searches each level, so an out-of-order entry is one
# it can fail to find rather than one it complains about.
$resources = @()
for ($i = 0; $i -lt $count; $i++) {
	$resources += [pscustomobject]@{ Type = $RT_ICON; Id = $i + 1; Data = $images[$i].Data }
}
$resources += [pscustomobject]@{ Type = $RT_GROUP_ICON; Id = 1; Data = $group.ToArray() }
if ($Version) {
	$resources += [pscustomobject]@{ Type = $RT_VERSION; Id = 1; Data = (New-VersionInfo $Version) }
}
$resources = @($resources | Sort-Object Type, Id)
$typeIds = @($resources | ForEach-Object { $_.Type } | Sort-Object -Unique)

# ---------------------------------------------------------------- layout

# Everything in a resource directory is addressed by an offset from the start
# of the section, so the whole tree has to be measured before any of it can be
# written. The order below is the order it is written in: the root directory,
# a directory of names per type, a directory of languages per name, the data
# entries, then the bytes themselves.

function Get-Key { param($Resource) "$($Resource.Type)/$($Resource.Id)" }

$typeDirAt = @{}
$nameDirAt = @{}
$dataEntryAt = @{}
$dataAt = @{}

$at = 16 + 8 * $typeIds.Count
foreach ($type in $typeIds) {
	$typeDirAt[$type] = $at
	$at += 16 + 8 * @($resources | Where-Object { $_.Type -eq $type }).Count
}
foreach ($resource in $resources) {
	$nameDirAt[(Get-Key $resource)] = $at
	$at += 16 + 8
}
foreach ($resource in $resources) {
	$dataEntryAt[(Get-Key $resource)] = $at
	$at += 16
}
foreach ($resource in $resources) {
	# Nothing requires the images be aligned, but an unaligned
	# BITMAPINFOHEADER is a misaligned read for whatever draws it later.
	$at = ($at + 7) -band (-bnot 7)
	$dataAt[(Get-Key $resource)] = $at
	$at += $resource.Data.Length
}
$sectionSize = $at

# ---------------------------------------------------------------- .rsrc

function Write-ResourceDirectory {
	param([System.IO.BinaryWriter]$Writer, [int]$Entries)

	$Writer.Write([uint32]0) # Characteristics
	$Writer.Write([uint32]0) # TimeDateStamp
	$Writer.Write([uint16]0) # MajorVersion
	$Writer.Write([uint16]0) # MinorVersion
	$Writer.Write([uint16]0) # NumberOfNamedEntries: ids throughout, no strings
	$Writer.Write([uint16]$Entries) # NumberOfIdEntries
}

function Write-ResourceEntry {
	param([System.IO.BinaryWriter]$Writer, [int]$Id, [int]$Offset, [switch]$Subdirectory)

	$Writer.Write([uint32]$Id)
	if ($Subdirectory) {
		# The top bit is what tells the loader this offset leads to another
		# directory rather than to a data entry. The L is not decoration:
		# PowerShell reads 0x80000000 as a *negative* 32-bit int, and the cast
		# below then refuses it.
		$Writer.Write([uint32]([int64]$Offset + 0x80000000L))
	} else {
		$Writer.Write([uint32]$Offset)
	}
}

$rsrc = New-Object System.IO.MemoryStream
$rsrcWriter = New-Object System.IO.BinaryWriter($rsrc)

Write-ResourceDirectory $rsrcWriter $typeIds.Count
foreach ($type in $typeIds) {
	Write-ResourceEntry $rsrcWriter $type $typeDirAt[$type] -Subdirectory
}

foreach ($type in $typeIds) {
	$ofType = @($resources | Where-Object { $_.Type -eq $type })
	Write-ResourceDirectory $rsrcWriter $ofType.Count
	foreach ($resource in $ofType) {
		Write-ResourceEntry $rsrcWriter $resource.Id $nameDirAt[(Get-Key $resource)] -Subdirectory
	}
}

foreach ($resource in $resources) {
	Write-ResourceDirectory $rsrcWriter 1
	Write-ResourceEntry $rsrcWriter $LANG_EN_US $dataEntryAt[(Get-Key $resource)]
}

foreach ($resource in $resources) {
	# IMAGE_RESOURCE_DATA_ENTRY. OffsetToData goes out as an offset into the
	# section and becomes an RVA through the relocation that names it.
	$rsrcWriter.Write([uint32]$dataAt[(Get-Key $resource)])
	$rsrcWriter.Write([uint32]$resource.Data.Length)
	$rsrcWriter.Write([uint32]0) # CodePage
	$rsrcWriter.Write([uint32]0) # Reserved
}

foreach ($resource in $resources) {
	while ($rsrc.Position -lt $dataAt[(Get-Key $resource)]) { $rsrcWriter.Write([byte]0) }
	$rsrcWriter.Write($resource.Data)
}
$rsrcWriter.Flush()

$section = $rsrc.ToArray()
if ($section.Length -ne $sectionSize) {
	throw "win-resources: wrote $($section.Length) bytes of .rsrc after measuring $sectionSize"
}

# ---------------------------------------------------------------- the object

$headers = 20 + 40 # IMAGE_FILE_HEADER, then the one section header
$relocations = 10 * $resources.Count

$name = New-Object byte[] 8
[System.Text.Encoding]::ASCII.GetBytes('.rsrc').CopyTo($name, 0)

$obj = New-Object System.IO.MemoryStream
$objWriter = New-Object System.IO.BinaryWriter($obj)

# IMAGE_FILE_HEADER
$objWriter.Write([uint16]0x8664) # Machine: amd64
$objWriter.Write([uint16]1)      # NumberOfSections
$objWriter.Write([uint32]0)      # TimeDateStamp: zero, so one icon is always the same bytes
$objWriter.Write([uint32]($headers + $section.Length + $relocations)) # PointerToSymbolTable
$objWriter.Write([uint32]1)      # NumberOfSymbols
$objWriter.Write([uint16]0)      # SizeOfOptionalHeader: none, this is an object
$objWriter.Write([uint16]0)      # Characteristics

# IMAGE_SECTION_HEADER
$objWriter.Write($name)
$objWriter.Write([uint32]0) # VirtualSize
$objWriter.Write([uint32]0) # VirtualAddress: the linker decides
$objWriter.Write([uint32]$section.Length) # SizeOfRawData
$objWriter.Write([uint32]$headers) # PointerToRawData
$objWriter.Write([uint32]($headers + $section.Length)) # PointerToRelocations
$objWriter.Write([uint32]0) # PointerToLinenumbers
$objWriter.Write([uint16]$resources.Count) # NumberOfRelocations
$objWriter.Write([uint16]0) # NumberOfLinenumbers
$objWriter.Write([uint32]0x40000040) # initialised data, readable

$objWriter.Write($section)

# One relocation per data entry, each pointing the linker at the OffsetToData
# field it has to turn into an RVA. ADDR32NB is "32-bit address, no base" --
# an offset from the image base, which is what an RVA is.
foreach ($resource in $resources) {
	$objWriter.Write([uint32]$dataEntryAt[(Get-Key $resource)]) # VirtualAddress
	$objWriter.Write([uint32]0)      # SymbolTableIndex: the .rsrc symbol below
	$objWriter.Write([uint16]0x0003) # IMAGE_REL_AMD64_ADDR32NB
}

# The only symbol: the section itself, at offset zero, which is what every one
# of those relocations is relative to.
$objWriter.Write($name)
$objWriter.Write([uint32]0) # Value
$objWriter.Write([int16]1)  # SectionNumber: the first and only section
$objWriter.Write([uint16]0) # Type
$objWriter.Write([byte]3)   # StorageClass: IMAGE_SYM_CLASS_STATIC
$objWriter.Write([byte]0)   # NumberOfAuxSymbols

# An empty string table is still four bytes saying so: a reader that finds
# nothing here reads whatever follows the symbols as a length.
$objWriter.Write([uint32]4)
$objWriter.Flush()

$out = Resolve-Full $OutPath
$dir = Split-Path -Parent $out
if ($dir -and -not (Test-Path $dir)) {
	New-Item -ItemType Directory -Path $dir -Force | Out-Null
}
[System.IO.File]::WriteAllBytes($out, $obj.ToArray())

$sizes = ($images | ForEach-Object {
		$side = $_.Width
		if ($side -eq 0) { $side = 256 }
		"${side}x${side}"
	}) -join ', '
$what = "icon $sizes"
if ($Version) { $what += ", version $Version" }
Write-Host "  resources $what -> $OutPath"
