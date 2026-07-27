// SPDX-Licence-Identifier: EUPL-1.2

package services_test

import (
	"sync"
	"time"

	core "dappco.re/go"
	coreio "dappco.re/go/io"
	coreprocess "dappco.re/go/process"
	"dappco.re/lthn/desktop/pkg/services"
)

type exampleProcess struct {
	once sync.Once
	done chan struct{}
	info coreprocess.Info
}

func (process *exampleProcess) Info() coreprocess.Info { return process.info }
func (process *exampleProcess) Output() string         { return "ready" }
func (process *exampleProcess) Wait() core.Result {
	<-process.done
	return core.Ok(nil)
}
func (process *exampleProcess) Shutdown() core.Result {
	process.info.Running = false
	process.info.Status = coreprocess.StatusKilled
	process.once.Do(func() { close(process.done) })
	return core.Ok(nil)
}

type exampleProcessRuntime struct {
	process *exampleProcess
}

func (runtime *exampleProcessRuntime) StartWithOptions(
	_ core.Context,
	_ coreprocess.RunOptions,
) core.Result {
	return core.Ok(services.ProcessHandle(runtime.process))
}

func (runtime *exampleProcessRuntime) Get(id string) core.Result {
	if id != runtime.process.info.ID {
		return core.Fail(core.E("example.Get", "not found", nil))
	}
	return core.Ok(services.ProcessHandle(runtime.process))
}

func (runtime *exampleProcessRuntime) Signal(id string, _ services.Signal) core.Result {
	if id != runtime.process.info.ID {
		return core.Fail(core.E("example.Signal", "not found", nil))
	}
	return core.Ok(true)
}

func (runtime *exampleProcessRuntime) Kill(id string) core.Result {
	if id != runtime.process.info.ID {
		return core.Fail(core.E("example.Kill", "not found", nil))
	}
	runtime.process.once.Do(func() { close(runtime.process.done) })
	return core.Ok(true)
}

func ExampleService() {
	medium := coreio.NewMemoryMedium()
	catalogue := services.NewMediumCatalogue(
		medium,
		"desktop/services/catalogue.json",
		services.DefaultLimits(),
	)
	_ = catalogue.Save(services.CatalogueDocument{
		Version: services.CatalogueVersion,
		Definitions: []services.Definition{{
			ID:                "api",
			DisplayName:       "Lethean API",
			Description:       "OpenAI-compatible local API.",
			Kind:              services.KindService,
			Command:           "lthn",
			Arguments:         []string{"serve"},
			RestartPolicy:     services.RestartNever,
			GracePeriodMillis: 5_000,
			Owner:             "lethean",
		}},
		PolicyOverrides: []services.PolicyOverride{},
		UpdatedAt:       "2026-07-27T12:00:00Z",
	})
	process := &exampleProcess{
		done: make(chan struct{}),
		info: coreprocess.Info{
			ID:        "proc-1",
			StartedAt: time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC),
			Running:   true,
			Status:    coreprocess.StatusRunning,
			PID:       4242,
		},
	}
	manager := services.NewService(services.Options{
		Process:   &exampleProcessRuntime{process: process},
		Catalogue: catalogue,
		Limits:    services.DefaultLimits(),
	})
	_ = manager.OnStartup(core.Background())

	started := manager.Start("api")
	stopped := manager.Stop("api")
	core.Println(
		started.Value.(services.Snapshot).State,
		stopped.Value.(services.Snapshot).State,
	)
	// Output: running stopped
}
