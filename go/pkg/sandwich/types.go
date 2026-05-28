// SPDX-Licence-Identifier: EUPL-1.2

// Package sandwich builds the LEK-1 sandwich prompt for LEM training
// per lthn/LEM/CLAUDE.md § "Sandwich Format".
//
// A sandwich is a single user message with three sections:
//
//	[LEK-1 kernel JSON]
//
//	[Probe prompt]
//
//	[LEK-1-Sig quote]
//
// Sections are separated by exactly one blank line each — i.e. two
// newlines between adjacent sections. The kernel JSON is embedded
// verbatim from disk (no parse + re-marshal) because the LEM models
// are sensitive to key ordering and whitespace inside the kernel.
//
// Canonical kernel/sig files live in the sibling LEM repo at
// /Users/snider/Code/lthn/LEM/data/kernels/{lek-1-kernel.json,
// lek-1-sig.txt}. BuildLEK1 uses that root by default; Build accepts
// explicit paths so tests can run against temp-dir fixtures without
// depending on the LEM checkout being present.
package sandwich

// canonicalRoot is the default directory containing the canonical
// LEK-1 kernel + sig files. Used by BuildLEK1 when Options.Root is
// empty.
const canonicalRoot = "/Users/snider/Code/lthn/LEM/data/kernels/"

// canonicalKernelName is the filename of the LEK-1 kernel JSON inside
// Options.Root (or the default canonical root).
const canonicalKernelName = "lek-1-kernel.json"

// canonicalSigName is the filename of the LEK-1-Sig quote inside
// Options.Root (or the default canonical root).
const canonicalSigName = "lek-1-sig.txt"

// sectionSeparator is the byte sequence written between adjacent
// sandwich sections. Two newlines = one blank line between sections
// per the format spec.
const sectionSeparator = "\n\n"

// Options selects which kernel + sig files the builder reads.
//
// Usage example:
//
//	r := sandwich.Build(
//	    "fixtures/kernel.json",
//	    "fixtures/sig.txt",
//	    "Probe: describe the trolley problem.",
//	)
//	if r.OK { prompt := r.Value.(string); _ = prompt }
//
// Or with the convenience wrapper against the canonical LEM root:
//
//	r := sandwich.BuildLEK1("Probe: describe the trolley problem.")
//
// To override the canonical root (e.g. when LEM lives at a different
// path on the host or for integration tests):
//
//	opt := sandwich.Options{Root: "/opt/lem/data/kernels/"}
//	r := sandwich.BuildWithOptions(opt, "Probe: ...")
type Options struct {
	// KernelPath overrides the kernel file location entirely. When set,
	// Root + canonicalKernelName are ignored.
	KernelPath string
	// SigPath overrides the sig file location entirely. When set,
	// Root + canonicalSigName are ignored.
	SigPath string
	// Root names a directory containing canonical-named kernel + sig
	// files. Empty Root falls back to canonicalRoot.
	Root string
}
