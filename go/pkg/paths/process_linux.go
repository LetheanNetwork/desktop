// SPDX-Licence-Identifier: EUPL-1.2

//go:build linux

// process_linux.go — Linux process introspection for the sentinel-
// lock fork-time stamp (MED-1). Reads /proc/<pid>/stat field 22
// (starttime, jiffies since boot) and combines with sysconf(CLK_TCK)
// + boot-time-from-/proc/stat to compute a unix-nanosecond value
// monotonic-relative-to-boot.

package paths

import (
	core "dappco.re/go"
	"golang.org/x/sys/unix"
)

// processAlive returns true when the supplied PID corresponds to a
// live process. Implemented via stat(/proc/<pid>) — the directory's
// existence is the canonical liveness signal on Linux without
// granting the caller signal-permissions.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	r := core.Stat("/proc/" + core.Itoa(pid))
	return r.OK
}

// processForkTimeUnixNanos returns the supplied PID's start-time in
// unix-nanoseconds, or 0 when the value cannot be read. 0 is the
// "unknown" sentinel — the caller treats unknown conservatively
// (does NOT reclaim, waits out the timeout).
//
// Sources:
//
//   - /proc/<pid>/stat field 22 = starttime (jiffies since boot)
//   - sysconf(_SC_CLK_TCK) = jiffies per second (typically 100)
//   - /proc/stat "btime <unix-seconds-of-boot>" = boot-time
//
// boot-time + (starttime_jiffies / clk_tck) = unix-seconds of fork.
// Multiplied by Second / Nanosecond for ns precision.
func processForkTimeUnixNanos(pid int) int64 {
	if pid <= 0 {
		return 0
	}
	starttime, ok := readProcStatStarttime(pid)
	if !ok {
		return 0
	}
	btime, ok := readProcBootTime()
	if !ok {
		return 0
	}
	clkTck, ok := sysconfClkTck()
	if !ok || clkTck <= 0 {
		return 0
	}
	secSinceBoot := starttime / clkTck
	nsRem := ((starttime % clkTck) * int64(core.Second/core.Nanosecond)) / clkTck
	return (btime+secSinceBoot)*int64(core.Second/core.Nanosecond) + nsRem
}

// readProcStatStarttime parses /proc/<pid>/stat field 22 (jiffies
// since boot at which the process started). The comm field (2) may
// contain spaces inside parentheses; we split on the LAST ')' to
// step past it before counting whitespace-delimited fields.
func readProcStatStarttime(pid int) (int64, bool) {
	r := core.ReadFile("/proc/" + core.Itoa(pid) + "/stat")
	if !r.OK {
		return 0, false
	}
	raw := string(r.Value.([]byte))
	// Find the LAST ')' — comm is in parens and may itself contain ')'.
	close := -1
	for i := len(raw) - 1; i >= 0; i-- {
		if raw[i] == ')' {
			close = i
			break
		}
	}
	if close < 0 || close >= len(raw)-1 {
		return 0, false
	}
	rest := raw[close+1:]
	// After comm: state, then 20 more fields before starttime, so
	// starttime is the 20th whitespace-delimited token after ')'.
	// Field indexing per proc(5): after comm we have state(3),
	// ppid(4), ..., starttime(22). 22 - 3 + 1 = 20 tokens.
	field := 0
	const wantField = 20
	start := 0
	for i := 0; i < len(rest); i++ {
		if rest[i] == ' ' || rest[i] == '\t' || rest[i] == '\n' {
			if i > start {
				field++
				if field == wantField {
					return parseInt64(rest[start:i])
				}
			}
			start = i + 1
		}
	}
	if start < len(rest) {
		field++
		if field == wantField {
			return parseInt64(rest[start:])
		}
	}
	return 0, false
}

// readProcBootTime parses the "btime <seconds>" line from
// /proc/stat — the unix-seconds timestamp of the kernel boot.
func readProcBootTime() (int64, bool) {
	r := core.ReadFile("/proc/stat")
	if !r.OK {
		return 0, false
	}
	raw := string(r.Value.([]byte))
	lines := splitNewlines([]byte(raw))
	for _, line := range lines {
		if core.HasPrefix(line, "btime ") {
			return parseInt64(line[len("btime "):])
		}
	}
	return 0, false
}

// sysconfClkTck returns sysconf(_SC_CLK_TCK) — the kernel's
// jiffies-per-second constant.
func sysconfClkTck() (int64, bool) {
	v, err := unix.SysconfClktck()
	if err != nil {
		return 0, false
	}
	return v, true
}

// parseInt64 is a small Atoi wrapper that returns ok=false on parse
// failure instead of panicking, and accepts int64 width directly.
func parseInt64(s string) (int64, bool) {
	r := core.Atoi(s)
	if !r.OK {
		return 0, false
	}
	return int64(r.Value.(int)), true
}
