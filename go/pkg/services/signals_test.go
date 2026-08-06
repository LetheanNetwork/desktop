// SPDX-Licence-Identifier: EUPL-1.2

package services

import (
	"syscall"
	"testing"

	core "dappco.re/go"
)

func TestResolveSignal_Good_MapsEveryNamedSignalOnUnix(t *testing.T) {
	for name, expected := range map[Signal]syscall.Signal{
		SignalTerminate: syscall.SIGTERM,
		SignalInterrupt: syscall.SIGINT,
		SignalHangup:    syscall.SIGHUP,
		SignalKill:      syscall.SIGKILL,
	} {
		result := resolveSignal(name, "linux")
		core.RequireTrue(t, result.OK, result.Error())
		core.AssertEqual(t, expected, result.Value.(syscall.Signal))
	}
}

func TestResolveSignal_Bad_RefusesAnUnknownName(t *testing.T) {
	result := resolveSignal(Signal("obliterate"), "linux")

	core.AssertFalse(t, result.OK, "an unknown name must be refused")
	core.AssertEqual(t, ErrorSignalUnknown, ErrorCodeOf(result))
}

// A number is the thing the named vocabulary exists to keep out. If "9" ever
// resolved, the boundary would be decorative.
func TestResolveSignal_Bad_RefusesASignalNumber(t *testing.T) {
	for _, numeric := range []string{"9", "15", "0", "-9"} {
		result := resolveSignal(Signal(numeric), "linux")

		core.AssertFalse(t, result.OK, core.Concat("expected ", numeric, " to be refused"))
		core.AssertEqual(t, ErrorSignalUnknown, ErrorCodeOf(result))
	}
}

func TestResolveSignal_Bad_RefusesAnEmptyName(t *testing.T) {
	result := resolveSignal(Signal(""), "linux")

	core.AssertFalse(t, result.OK, "an empty name must be refused")
	core.AssertEqual(t, ErrorSignalUnknown, ErrorCodeOf(result))
}

// Windows has no signals. Ending a process is meaningful there; reloading it
// is not, and reporting success for a reload that did not happen is worse
// than refusing.
func TestResolveSignal_Ugly_WindowsEndsButWillNotReload(t *testing.T) {
	for _, name := range []Signal{SignalTerminate, SignalKill} {
		result := resolveSignal(name, "windows")
		core.AssertTrue(t, result.OK, core.Concat(string(name), " must be accepted on Windows"))
	}

	for _, name := range []Signal{SignalInterrupt, SignalHangup} {
		result := resolveSignal(name, "windows")

		core.AssertFalse(t, result.OK, core.Concat(string(name), " must be refused on Windows"))
		core.AssertEqual(t, ErrorSignalUnsupported, ErrorCodeOf(result))
	}
}

func TestResolveSignal_Ugly_EmptyGoosFallsBackToThisHost(t *testing.T) {
	// Not asserting which host — asserting it resolves rather than treating
	// "" as an unknown platform and refusing everything.
	result := resolveSignal(SignalTerminate, "")

	core.AssertTrue(t, result.OK, result.Error())
}

// TestDeliverNamedSignal_Bad_UnknownNameNeverReachesService proves the
// resolveSignal failure short-circuits deliverNamedSignal before it
// ever touches the *coreprocess.Service — passing nil for service
// would panic if the guard were missing.
func TestDeliverNamedSignal_Bad_UnknownNameNeverReachesService(t *testing.T) {
	result := deliverNamedSignal(nil, "any-id", Signal("obliterate"))

	core.AssertFalse(t, result.OK, "an unknown signal name must be refused before reaching the service")
	core.AssertEqual(t, ErrorSignalUnknown, ErrorCodeOf(result))
}

func TestValidSignal_Good_KnowsTheVocabulary(t *testing.T) {
	for _, name := range []Signal{SignalTerminate, SignalInterrupt, SignalHangup, SignalKill} {
		core.AssertTrue(t, validSignal(name), core.Concat(string(name), " must be valid"))
	}

	for _, name := range []Signal{"", "9", "sigterm", "TERM", "obliterate"} {
		core.AssertFalse(t, validSignal(name), core.Concat(string(name), " must not be valid"))
	}
}
