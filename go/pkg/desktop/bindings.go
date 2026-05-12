// SPDX-Licence-Identifier: EUPL-1.2

// Wails-binding adapter services. Each type wraps one lthn domain
// package with binding-friendly `(T, error)` method signatures so
// Wails' TypeScript binding generator produces clean typed access
// from the WebView.
//
// Why adapters: our internal Go contracts return core.Result for the
// uniform-action-bus discipline (per AX-4). Wails' TS generator
// prefers idiomatic Go `(value, error)`. The adapters translate at
// the boundary — no semantics lost, the TS shape is clean.
//
// Wails service interface lined up with Core's lifecycle:
//
//	ServiceName() string                                      ↔ core service name
//	ServiceStartup(ctx, ServiceOptions) error                 ↔ OnStartup(ctx) Result
//	ServiceShutdown() error                                   ↔ OnShutdown(ctx) Result
//
// Snider 2026-05-12: "Core is compatible with Wails on a fundamental
// level" — the lifecycle shape matches, only the return type
// differs (Result vs error).
//
// Usage example: see desktop.NewService — the adapters are passed
// to application.NewService(...) and registered in Options.Services.
package desktop

import (
	"context"
	"errors"

	core "dappco.re/go"
	"dappco.re/go/inference"
	"dappco.re/lthn/desktop/pkg/firstlaunch"
	"dappco.re/lthn/desktop/pkg/models"
	"dappco.re/lthn/desktop/pkg/runner"
	"dappco.re/lthn/desktop/pkg/sessions"
	lthnservices "dappco.re/lthn/desktop/pkg/services"
	"dappco.re/lthn/desktop/pkg/telemetry"
	"dappco.re/lthn/desktop/pkg/validator"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// resultToError unpacks a core.Result into the idiomatic (T, error)
// shape the TS bindings consume.
func resultToError[T any](r core.Result) (T, error) {
	var zero T
	if !r.OK {
		if err, ok := r.Value.(error); ok {
			return zero, err
		}
		return zero, errors.New(r.Error())
	}
	v, ok := r.Value.(T)
	if !ok {
		return zero, errors.New("unexpected value type from core.Result")
	}
	return v, nil
}

// ---------------------------------------------------------------------
// RunnerService — talk surface for the WebView. Binds to ai.* in TS.
// ---------------------------------------------------------------------

type RunnerService struct {
	svc *runner.Service
}

func NewRunnerService(svc *runner.Service) *RunnerService { return &RunnerService{svc: svc} }

func (r *RunnerService) ServiceName() string { return "Runner" }
func (r *RunnerService) ServiceStartup(_ context.Context, _ application.ServiceOptions) error {
	return nil
}
func (r *RunnerService) ServiceShutdown() error { return nil }

// Generate returns the assistant reply for a single-prompt request.
func (r *RunnerService) Generate(prompt string) (string, error) {
	return resultToError[string](r.svc.Generate(prompt))
}

// Chat takes a full message history and returns the assistant reply.
func (r *RunnerService) Chat(messages []inference.Message) (string, error) {
	return resultToError[string](r.svc.Chat(messages))
}

// Models lists configured route names.
func (r *RunnerService) Models() ([]string, error) {
	return resultToError[[]string](r.svc.Models())
}

// ---------------------------------------------------------------------
// SessionsService — chat history persisted via go-store.
// ---------------------------------------------------------------------

type SessionsService struct {
	core *core.Core
}

func NewSessionsService(c *core.Core) *SessionsService { return &SessionsService{core: c} }

func (s *SessionsService) ServiceName() string { return "Sessions" }
func (s *SessionsService) ServiceStartup(_ context.Context, _ application.ServiceOptions) error {
	return nil
}
func (s *SessionsService) ServiceShutdown() error { return nil }

func (s *SessionsService) Create(title string) (string, error) {
	return resultToError[string](sessions.Create(s.core, title))
}

func (s *SessionsService) Append(id, role, content string) error {
	r := sessions.Append(s.core, id, role, content)
	if !r.OK {
		return errors.New(r.Error())
	}
	return nil
}

func (s *SessionsService) Read(id string) ([]inference.Message, error) {
	return resultToError[[]inference.Message](sessions.Read(s.core, id))
}

func (s *SessionsService) List() ([]sessions.SessionInfo, error) {
	return resultToError[[]sessions.SessionInfo](sessions.List(s.core))
}

// ---------------------------------------------------------------------
// ConfigService — YAML config read/write through core/config actions.
// ---------------------------------------------------------------------

type ConfigService struct {
	core *core.Core
}

func NewConfigService(c *core.Core) *ConfigService { return &ConfigService{core: c} }

func (s *ConfigService) ServiceName() string { return "Config" }
func (s *ConfigService) ServiceStartup(_ context.Context, _ application.ServiceOptions) error {
	return nil
}
func (s *ConfigService) ServiceShutdown() error { return nil }

func (s *ConfigService) Get(key string) (any, error) {
	r := s.core.Action("config.get").Run(context.Background(), core.NewOptions(
		core.Option{Key: "key", Value: key},
	))
	if !r.OK {
		return nil, errors.New(r.Error())
	}
	return r.Value, nil
}

func (s *ConfigService) Set(key, value string) error {
	r := s.core.Action("config.set").Run(context.Background(), core.NewOptions(
		core.Option{Key: "key", Value: key},
		core.Option{Key: "value", Value: value},
	))
	if !r.OK {
		return errors.New(r.Error())
	}
	commit := s.core.Action("config.commit").Run(context.Background(), core.NewOptions())
	if !commit.OK {
		return errors.New(commit.Error())
	}
	return nil
}

func (s *ConfigService) All() (any, error) {
	r := s.core.Action("config.all").Run(context.Background(), core.NewOptions())
	if !r.OK {
		return nil, errors.New(r.Error())
	}
	return r.Value, nil
}

// ---------------------------------------------------------------------
// ModelsService — local snapshot directory listing.
// ---------------------------------------------------------------------

type ModelsService struct{}

func NewModelsService() *ModelsService { return &ModelsService{} }

func (s *ModelsService) ServiceName() string { return "Models" }
func (s *ModelsService) ServiceStartup(_ context.Context, _ application.ServiceOptions) error {
	return nil
}
func (s *ModelsService) ServiceShutdown() error { return nil }

func (s *ModelsService) List() ([]models.Entry, error) {
	return resultToError[[]models.Entry](models.List())
}

// ---------------------------------------------------------------------
// FirstLaunchService — wizard-driving detector.
// ---------------------------------------------------------------------

type FirstLaunchService struct{}

func NewFirstLaunchService() *FirstLaunchService { return &FirstLaunchService{} }

func (s *FirstLaunchService) ServiceName() string { return "FirstLaunch" }
func (s *FirstLaunchService) ServiceStartup(_ context.Context, _ application.ServiceOptions) error {
	return nil
}
func (s *FirstLaunchService) ServiceShutdown() error { return nil }

func (s *FirstLaunchService) Detect() (firstlaunch.State, error) {
	return resultToError[firstlaunch.State](firstlaunch.Detect(nil))
}

// ---------------------------------------------------------------------
// ValidatorService — remote endpoint probe.
// ---------------------------------------------------------------------

type ValidatorService struct{}

func NewValidatorService() *ValidatorService { return &ValidatorService{} }

func (s *ValidatorService) ServiceName() string { return "Validator" }
func (s *ValidatorService) ServiceStartup(_ context.Context, _ application.ServiceOptions) error {
	return nil
}
func (s *ValidatorService) ServiceShutdown() error { return nil }

func (s *ValidatorService) Endpoint(baseURL string) (validator.EndpointInfo, error) {
	return resultToError[validator.EndpointInfo](validator.Endpoint(baseURL))
}

// ---------------------------------------------------------------------
// TelemetryService — process metrics.
// ---------------------------------------------------------------------

type TelemetryService struct{}

func NewTelemetryService() *TelemetryService { return &TelemetryService{} }

func (s *TelemetryService) ServiceName() string { return "Telemetry" }
func (s *TelemetryService) ServiceStartup(_ context.Context, _ application.ServiceOptions) error {
	return nil
}
func (s *TelemetryService) ServiceShutdown() error { return nil }

func (s *TelemetryService) Sample() (telemetry.Reading, error) {
	return resultToError[telemetry.Reading](telemetry.Sample())
}

// ---------------------------------------------------------------------
// EnvService — wraps app.Env (OS info / dark-mode / file manager).
// ---------------------------------------------------------------------

// EnvInfo mirrors application.EnvironmentInfo with JSON-stable
// field names. PlatformInfo is left as map[string]any because
// the field shapes vary per OS.
type EnvInfo struct {
	OS           string         `json:"os"`
	Arch         string         `json:"arch"`
	Debug        bool           `json:"debug"`
	DarkMode     bool           `json:"dark_mode"`
	PlatformInfo map[string]any `json:"platform_info,omitempty"`
	OSName       string         `json:"os_name,omitempty"`
	OSVersion    string         `json:"os_version,omitempty"`
}

type EnvService struct {
	// app is set by desktop.Run() AFTER application.New returns —
	// EnvironmentManager isn't available pre-construction.
	app *application.App
}

func NewEnvService() *EnvService { return &EnvService{} }

func (s *EnvService) ServiceName() string { return "Env" }
func (s *EnvService) ServiceStartup(_ context.Context, _ application.ServiceOptions) error {
	return nil
}
func (s *EnvService) ServiceShutdown() error { return nil }

// Info returns a snapshot of the runtime environment.
func (s *EnvService) Info() (EnvInfo, error) {
	if s.app == nil {
		return EnvInfo{}, errors.New("env service not yet attached to wails app")
	}
	info := s.app.Env.Info()
	out := EnvInfo{
		OS:           info.OS,
		Arch:         info.Arch,
		Debug:        info.Debug,
		DarkMode:     s.app.Env.IsDarkMode(),
		PlatformInfo: info.PlatformInfo,
	}
	if info.OSInfo != nil {
		out.OSName = info.OSInfo.Name
		out.OSVersion = info.OSInfo.Version
	}
	return out, nil
}

// IsDarkMode returns true when the OS theme is dark. Theme changes
// are re-broadcast as "lthn:theme" events from desktop.Run().
func (s *EnvService) IsDarkMode() (bool, error) {
	if s.app == nil {
		return false, errors.New("env service not yet attached to wails app")
	}
	return s.app.Env.IsDarkMode(), nil
}

// OpenFileManager opens the OS-native file manager at path. When
// selectFile is true and path points at a file, the manager
// highlights it (Finder, Explorer; Linux varies by DE).
//
// Common use: "Show models folder" → Finder opens
// ~/Lethean/conf/models/. "Reveal this session" → Finder opens
// the directory with the file highlighted.
func (s *EnvService) OpenFileManager(path string, selectFile bool) error {
	if s.app == nil {
		return errors.New("env service not yet attached to wails app")
	}
	return s.app.Env.OpenFileManager(path, selectFile)
}

// ---------------------------------------------------------------------
// LifecycleService — OS service install/start/stop for the wizard.
// ---------------------------------------------------------------------

type LifecycleService struct{}

func NewLifecycleService() *LifecycleService { return &LifecycleService{} }

func (s *LifecycleService) ServiceName() string { return "Lifecycle" }
func (s *LifecycleService) ServiceStartup(_ context.Context, _ application.ServiceOptions) error {
	return nil
}
func (s *LifecycleService) ServiceShutdown() error { return nil }

func (s *LifecycleService) Registry() []lthnservices.Entry { return lthnservices.Registry() }

func (s *LifecycleService) Install(name string) error {
	r := lthnservices.Install(name)
	if !r.OK {
		return errors.New(r.Error())
	}
	return nil
}

func (s *LifecycleService) Uninstall(name string) error {
	r := lthnservices.Uninstall(name)
	if !r.OK {
		return errors.New(r.Error())
	}
	return nil
}

func (s *LifecycleService) Start(name string) error {
	r := lthnservices.Start(name)
	if !r.OK {
		return errors.New(r.Error())
	}
	return nil
}

func (s *LifecycleService) Stop(name string) error {
	r := lthnservices.Stop(name)
	if !r.OK {
		return errors.New(r.Error())
	}
	return nil
}

func (s *LifecycleService) Status(name string) (string, error) {
	return resultToError[string](lthnservices.Status(name))
}
