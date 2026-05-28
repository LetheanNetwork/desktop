// SPDX-Licence-Identifier: EUPL-1.2

package training_test

import (
	core "dappco.re/go"

	"dappco.re/lthn/desktop/pkg/clbpl"
	"dappco.re/lthn/desktop/pkg/training"
)

// ExampleService_Run shows the canonical wire shape: a Core handle,
// Options scoped to a single subject + small probe budget, register
// the action surface, then drive the rotation against a FixtureRunner
// so the example runs without a real go-mlx runner present.
//
// In production code the FixtureRunner is replaced with the go-mlx /
// llama.cpp / lthn-mlx Runner implementation; the Service surface
// doesn't change.
func ExampleService_Run() {
	c := core.New()

	svc := training.NewService(c, training.Options{
		Subjects:         []string{"english"},
		ProbesPerSubject: 1,
		CLBPLOptions: clbpl.Options{
			Window:           4,
			GrokThreshold:    0.15,
			GrokConfirmPeaks: 2,
		},
		Epoch2MaxSteps: 64,
	})
	_ = svc.Register(c)

	runner := training.NewFixtureRunner(training.FixtureConfig{
		Model:     "fixture-e2b",
		Tier:      0,
		Substrate: "CONT",
	})

	res := svc.Run(c.Context(), runner)
	if !res.OK {
		return
	}
	// res.Value is a RunCompleteEvent — TotalSubjects, GrokedSubjects,
	// ElapsedSeconds, Cancelled. Subscribers receive the typed event
	// via c.RegisterAction during the run.
	_ = res.Value.(training.RunCompleteEvent)
}
