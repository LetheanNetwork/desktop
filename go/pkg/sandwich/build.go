// SPDX-Licence-Identifier: EUPL-1.2

// build.go — sandwich assembly. Reads kernel + sig from disk verbatim,
// concatenates with the two-newline separator per the LEK-1 sandwich
// format, returns the full prompt string via core.Result.
//
// The kernel + sig bytes are written into the result with NO whitespace
// normalisation — any trailing newlines on the source files survive
// into the sandwich verbatim. This is load-bearing: LEM models train
// against a specific byte sequence and silent whitespace drift breaks
// recall. The byte-stability invariant is locked in by the SHA-256
// fixture test in build_test.go.

package sandwich

import (
	core "dappco.re/go"
)

// Build assembles a sandwich prompt from explicit kernel + sig paths.
//
// Reads kernelPath and sigPath via core.ReadFile, concatenates the
// three sections (kernel || sep || probe || sep || sig) and returns
// the result as a string in core.Ok. An empty probe is permitted —
// the middle section is empty in that case (sandwich shape stays
// intact, just with no probe content between the two separators).
//
// Returns core.Fail with a "sandwich.Build" error code when either
// file is missing or unreadable; the error message names the offending
// path so the caller can surface it.
//
// Usage example:
//
//	r := sandwich.Build(
//	    "fixtures/kernel.json",
//	    "fixtures/sig.txt",
//	    "Probe: describe the trolley problem.",
//	)
//	if !r.OK {
//	    core.Print(r.Value.(error).Error())
//	    return
//	}
//	prompt := r.Value.(string)
func Build(kernelPath, sigPath, probe string) core.Result {
	if st := core.Stat(kernelPath); !st.OK {
		return core.Fail(core.E("sandwich.Build",
			"kernel file not found at "+kernelPath, nil))
	}
	if st := core.Stat(sigPath); !st.OK {
		return core.Fail(core.E("sandwich.Build",
			"sig file not found at "+sigPath, nil))
	}

	kR := core.ReadFile(kernelPath)
	if !kR.OK {
		return core.Fail(core.E("sandwich.Build",
			"reading kernel file at "+kernelPath+" failed",
			kR.Value.(error)))
	}
	sR := core.ReadFile(sigPath)
	if !sR.OK {
		return core.Fail(core.E("sandwich.Build",
			"reading sig file at "+sigPath+" failed",
			sR.Value.(error)))
	}

	kernel, _ := kR.Value.([]byte)
	sig, _ := sR.Value.([]byte)

	// Pre-size to avoid grow-and-copy: kernel + sep + probe + sep + sig.
	buf := make([]byte, 0, len(kernel)+len(sectionSeparator)+len(probe)+len(sectionSeparator)+len(sig))
	buf = append(buf, kernel...)
	buf = append(buf, []byte(sectionSeparator)...)
	buf = append(buf, []byte(probe)...)
	buf = append(buf, []byte(sectionSeparator)...)
	buf = append(buf, sig...)

	return core.Ok(string(buf))
}

// BuildWithOptions resolves the kernel + sig paths from opt then calls
// Build. Empty opt.Root falls back to the canonical LEM root. Explicit
// opt.KernelPath / opt.SigPath override the root-based resolution.
//
// Usage example:
//
//	opt := sandwich.Options{Root: tmpDir}
//	r := sandwich.BuildWithOptions(opt, "Probe: ...")
func BuildWithOptions(opt Options, probe string) core.Result {
	root := opt.Root
	if root == "" {
		root = canonicalRoot
	}
	kernelPath := opt.KernelPath
	if kernelPath == "" {
		kernelPath = core.PathJoin(root, canonicalKernelName)
	}
	sigPath := opt.SigPath
	if sigPath == "" {
		sigPath = core.PathJoin(root, canonicalSigName)
	}
	return Build(kernelPath, sigPath, probe)
}

// BuildLEK1 is the convenience wrapper using the canonical LEM kernel
// root (/Users/snider/Code/lthn/LEM/data/kernels/). Equivalent to:
//
//	BuildWithOptions(Options{}, probe)
//
// Usage example:
//
//	r := sandwich.BuildLEK1("Probe: describe the trolley problem.")
//	if !r.OK { return r }
//	prompt := r.Value.(string)
func BuildLEK1(probe string) core.Result {
	return BuildWithOptions(Options{}, probe)
}
