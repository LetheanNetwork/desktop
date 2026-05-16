// SPDX-Licence-Identifier: EUPL-1.2

//go:build darwin

// process_darwin.go — Darwin process introspection for the
// sentinel-lock fork-time stamp (MED-1). Uses sysctl(KERN_PROC_PID)
// to read the process's KP_PROC.P_STARTTIME timeval; converted to
// unix-nanoseconds for stable comparison with the value cached in
// the sentinel file.

package paths

import (
	"syscall"
	"unsafe"

	core "dappco.re/go"
	"golang.org/x/sys/unix"
)

// Darwin sysctl identifiers — not exported by x/sys/unix today
// (logged as a CoreGO gap path-adjacent). Values per
// /usr/include/sys/sysctl.h.
const (
	darwinKernProc    = 14 // KERN_PROC
	darwinKernProcPID = 1  // KERN_PROC_PID
)

// processAlive returns true when the supplied PID corresponds to a
// live process. Implemented via sysctl(KERN_PROC_PID) — preferable
// to kill(pid, 0) on Darwin where signal-0 succeeds against zombies
// the user has no right to signal.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	_, err := sysctlKernProc(pid)
	return err == nil
}

// processForkTimeUnixNanos returns the supplied PID's start-time in
// unix-nanoseconds, or 0 when the value cannot be read. 0 is the
// sentinel for "unknown" — the caller treats unknown conservatively
// (does NOT reclaim, waits out the timeout).
//
// MED-1: monotonic-relative-to-boot is the goal; this value comes
// from the kernel's bookkeeping of when the process was forked, so
// system-clock skew (NTP step, manual date change) does not falsely
// invalidate a live lock.
func processForkTimeUnixNanos(pid int) int64 {
	if pid <= 0 {
		return 0
	}
	kp, err := sysctlKernProc(pid)
	if err != nil {
		return 0
	}
	// KinfoProc.Proc.P_starttime is a Timeval (Sec int64, Usec int32).
	sec := int64(kp.Proc.P_starttime.Sec)
	usec := int64(kp.Proc.P_starttime.Usec)
	return sec*int64(core.Second/core.Nanosecond) + usec*1000
}

// sysctlKernProc reads the KinfoProc record for pid via
// sysctl({CTL_KERN, KERN_PROC, KERN_PROC_PID, pid}). On a dead /
// unknown PID the call returns ESRCH which the caller treats as
// "not alive".
func sysctlKernProc(pid int) (*unix.KinfoProc, error) {
	mib := []int32{unix.CTL_KERN, darwinKernProc, darwinKernProcPID, int32(pid)}
	var kp unix.KinfoProc
	size := unsafe.Sizeof(kp)
	_, _, errno := syscall.Syscall6(
		syscall.SYS___SYSCTL,
		uintptr(unsafe.Pointer(&mib[0])),
		uintptr(len(mib)),
		uintptr(unsafe.Pointer(&kp)),
		uintptr(unsafe.Pointer(&size)),
		0, 0,
	)
	if errno != 0 {
		return nil, errno
	}
	if size == 0 {
		// Empty record — PID does not exist.
		return nil, syscall.ESRCH
	}
	return &kp, nil
}
