// Bindings for the winmm waveOut API, which is how PCM reaches a named
// playback device without cgo.
//
// waveOut is the oldest of the three Windows audio APIs and the only one
// callable through plain syscalls: WASAPI and DirectSound are COM, which needs
// interface vtables and a good deal of ceremony to reach from Go. Everything
// here goes through the Windows audio engine either way, so the engine
// resamples the voice's rate to whatever the device is configured for and the
// age of the API costs nothing but a few milliseconds of latency.
package audio

import (
	"fmt"
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/windows"
)

// NewLazySystemDLL resolves out of System32 rather than the executable's
// directory, so a winmm.dll dropped beside the binary is not picked up.
var (
	winmm = windows.NewLazySystemDLL("winmm.dll")

	procWaveOutGetNumDevs      = winmm.NewProc("waveOutGetNumDevs")
	procWaveOutGetDevCaps      = winmm.NewProc("waveOutGetDevCapsW")
	procWaveOutOpen            = winmm.NewProc("waveOutOpen")
	procWaveOutClose           = winmm.NewProc("waveOutClose")
	procWaveOutPrepareHeader   = winmm.NewProc("waveOutPrepareHeader")
	procWaveOutUnprepareHeader = winmm.NewProc("waveOutUnprepareHeader")
	procWaveOutWrite           = winmm.NewProc("waveOutWrite")
	procWaveOutReset           = winmm.NewProc("waveOutReset")
	procWaveOutGetErrorText    = winmm.NewProc("waveOutGetErrorTextW")

	procWaveInGetNumDevs = winmm.NewProc("waveInGetNumDevs")
	procWaveInGetDevCaps = winmm.NewProc("waveInGetDevCapsW")
)

const (
	mmsyserrNoError = 0

	// waveMapper is the device index meaning "whatever the user's default
	// output is", which is what monitoring plays to.
	waveMapper = ^uintptr(0) // (UINT)-1

	waveFormatPCM = 1
	callbackEvent = 0x00050000

	// WHDR_INQUEUE is the only header flag anything here reads: it is set by
	// waveOutWrite and cleared when the driver is done with the buffer, which
	// is exactly the question the writer asks.
	whdrInQueue = 0x00000010

	maxPnameLen    = 32  // WCHARs, including the terminator
	maxErrorLength = 256 // WCHARs
)

// waveFormatEx describes the PCM the device is opened with. cbSize stays zero:
// that field only matters for the extensible formats this never uses.
type waveFormatEx struct {
	wFormatTag      uint16
	nChannels       uint16
	nSamplesPerSec  uint32
	nAvgBytesPerSec uint32
	nBlockAlign     uint16
	wBitsPerSample  uint16
	cbSize          uint16
}

// waveHdr is one queued buffer. The driver writes dwFlags back from its own
// thread when the buffer drains, which is why flags() reads it atomically.
type waveHdr struct {
	lpData          uintptr // set to &buf[0]; see queue() for why that is safe
	dwBufferLength  uint32
	dwBytesRecorded uint32
	dwUser          uintptr
	dwFlags         uint32
	dwLoops         uint32
	lpNext          uintptr
	reserved        uintptr
}

func (h *waveHdr) flags() uint32 {
	return atomic.LoadUint32(&h.dwFlags)
}

// waveOutCaps is WAVEOUTCAPSW. szPname is MAXPNAMELEN wide characters, so
// device names arrive truncated to 31 characters — long enough to tell the
// cables apart, which is all the matching here needs.
type waveOutCaps struct {
	wMid           uint16
	wPid           uint16
	vDriverVersion uint32
	szPname        [maxPnameLen]uint16
	dwFormats      uint32
	wChannels      uint16
	wReserved1     uint16
	dwSupport      uint32
}

// waveInCaps is WAVEINCAPSW, identical to the output caps up to the last field.
type waveInCaps struct {
	wMid           uint16
	wPid           uint16
	vDriverVersion uint32
	szPname        [maxPnameLen]uint16
	dwFormats      uint32
	wChannels      uint16
	wReserved1     uint16
}

// playbackDevices lists the waveOut device names, indexed by device ID.
func playbackDevices() []string {
	n, _, _ := procWaveOutGetNumDevs.Call()
	names := make([]string, 0, n)
	for i := uintptr(0); i < n; i++ {
		var caps waveOutCaps
		ret, _, _ := procWaveOutGetDevCaps.Call(i,
			uintptr(unsafe.Pointer(&caps)), unsafe.Sizeof(caps))
		if ret != mmsyserrNoError {
			// A device that will not describe itself is one this cannot
			// route to either; hold its slot so IDs stay the indices.
			names = append(names, "")
			continue
		}
		names = append(names, windows.UTF16ToString(caps.szPname[:]))
	}
	return names
}

// captureDevices lists the waveIn device names. Nothing is recorded here; the
// list is only read to confirm the cable's capture half is present and to
// report the name a user has to pick in other applications.
func captureDevices() []string {
	n, _, _ := procWaveInGetNumDevs.Call()
	names := make([]string, 0, n)
	for i := uintptr(0); i < n; i++ {
		var caps waveInCaps
		ret, _, _ := procWaveInGetDevCaps.Call(i,
			uintptr(unsafe.Pointer(&caps)), unsafe.Sizeof(caps))
		if ret != mmsyserrNoError {
			names = append(names, "")
			continue
		}
		names = append(names, windows.UTF16ToString(caps.szPname[:]))
	}
	return names
}

// mmError turns an MMRESULT into an error carrying the driver's own wording,
// falling back to the bare code for results winmm has no text for.
func mmError(op string, ret uintptr) error {
	if ret == mmsyserrNoError {
		return nil
	}
	var buf [maxErrorLength]uint16
	textRet, _, _ := procWaveOutGetErrorText.Call(ret,
		uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if textRet == mmsyserrNoError {
		if msg := windows.UTF16ToString(buf[:]); msg != "" {
			return fmt.Errorf("%s: %s", op, msg)
		}
	}
	return fmt.Errorf("%s: winmm error %d", op, ret)
}
