// SPDX-Licence-Identifier: EUPL-1.2

package services

import (
	"runtime"
	"syscall"

	core "dappco.re/go"
	coreprocess "dappco.re/go/process"
)

// resolveSignal maps a named signal to the value this platform understands.
//
// Deliberately switched on runtime.GOOS rather than split across build-tagged
// files. The Windows rule — that interrupt and hangup have no meaning there —
// is a rule worth a test, and under build tags that test could only ever run
// on Windows, which is to say never. Here it runs everywhere.
func resolveSignal(name Signal, goos string) core.Result {
	if goos == "" {
		goos = runtime.GOOS
	}

	sig, known := signalNames[name]
	if !known {
		return core.Fail(&Failure{
			Code:      ErrorSignalUnknown,
			Operation: "services.resolveSignal",
			Message:   "That is not a signal this manager sends.",
		})
	}

	// Windows has no signals. Go will accept syscall.SIGTERM and SIGKILL as
	// "end this process", and silently do nothing useful for the rest — which
	// is the worst outcome: a reload button that reports success and reloads
	// nothing. Refuse instead.
	if goos == "windows" && !windowsDelivers[name] {
		return core.Fail(&Failure{
			Code:      ErrorSignalUnsupported,
			Operation: "services.resolveSignal",
			Message:   core.Concat("Windows cannot send ", string(name), " to a process."),
		})
	}

	return core.Ok(sig)
}

// signalNames is the whole vocabulary. Adding to it is a deliberate act; a
// caller cannot widen it by passing a number.
var signalNames = map[Signal]syscall.Signal{
	SignalTerminate: syscall.SIGTERM,
	SignalInterrupt: syscall.SIGINT,
	SignalHangup:    syscall.SIGHUP,
	SignalKill:      syscall.SIGKILL,
}

// windowsDelivers records which of the four mean anything on Windows.
var windowsDelivers = map[Signal]bool{
	SignalTerminate: true,
	SignalKill:      true,
}

// validSignal reports whether a name is in the vocabulary at all, without
// deciding whether this platform can deliver it.
func validSignal(name Signal) bool {
	_, known := signalNames[name]
	return known
}

// deliverNamedSignal is the single place a named signal becomes a kernel
// constant, and the only reason this file imports syscall.
//
// Everything above it — the manager, the runtime interface, the Wails
// boundary, the renderer — speaks names. That is not decoration: it is what
// makes "the renderer cannot choose a signal number" a structural fact rather
// than a check somebody has to remember to write.
func deliverNamedSignal(service *coreprocess.Service, id string, name Signal) core.Result {
	resolved := resolveSignal(name, "")
	if !resolved.OK {
		return resolved
	}
	return service.Signal(id, resolved.Value.(syscall.Signal))
}
