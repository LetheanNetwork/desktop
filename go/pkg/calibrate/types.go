// SPDX-Licence-Identifier: EUPL-1.2

package calibrate

// MachineReport is the parsed output of `lthn-mlx discover --json` — the
// inference capability + memory envelope of the machine the desktop is
// running on. It is the first step of a calibration: discover what this
// hardware can do before tuning a model to it.
//
// Zero values mean "the field was absent from the report", not a real
// zero — discover always populates Runtime + Device + Available on a
// supported backend.
//
// Usage example:
//
//	r := calibrate.NewService(c).Discover()
//	if r.OK {
//	    rep := r.Value.(calibrate.MachineReport)
//	    _ = rep.Device.MemorySize // bytes of unified / device memory
//	}
type MachineReport struct {
	Runtime      RuntimeInfo  `json:"runtime"`
	Device       DeviceInfo   `json:"device"`
	Available    bool         `json:"available"`
	Capabilities []Capability `json:"capabilities"`
}

// RuntimeInfo is the active inference backend + its labels (memory
// budget strings, working-set bytes). Backend is "metal" on Apple
// silicon; native_runtime distinguishes the go-mlx native path from a
// shelled MLX-Python fallback.
type RuntimeInfo struct {
	Backend       string            `json:"backend"`
	Device        string            `json:"device"`
	NativeRuntime bool              `json:"native_runtime"`
	Labels        map[string]string `json:"labels,omitempty"`
}

// DeviceInfo is the physical accelerator's name + memory envelope. The
// three byte counts bound what models + context lengths fit; the
// machine_hash label (under Labels) keys tuned profiles to this exact
// machine so a profile from a different box is never mis-applied.
type DeviceInfo struct {
	Name                         string            `json:"name"`
	Architecture                 string            `json:"architecture"`
	MaxBufferLength              int64             `json:"max_buffer_length"`
	MaxRecommendedWorkingSetSize int64             `json:"max_recommended_working_set_size"`
	MemorySize                   int64             `json:"memory_size"`
	Labels                       map[string]string `json:"labels,omitempty"`
}

// Capability is one runtime feature discover reports as supported or
// not — e.g. {id:"runtime.autotune", group:"runtime", status:"supported"}.
// The Calibrate flow checks for "runtime.autotune" before offering the
// unattended tuning sweep.
type Capability struct {
	ID     string `json:"id"`
	Group  string `json:"group"`
	Status string `json:"status"`
}

// Supports reports whether the report lists capability id with status
// "supported". Used to gate Calibrate steps on what the backend can do.
//
// Usage example:
//
//	if rep.Supports("runtime.autotune") { /* offer Calibrate */ }
func (r MachineReport) Supports(id string) bool {
	for _, c := range r.Capabilities {
		if c.ID == id {
			return c.Status == "supported"
		}
	}
	return false
}
