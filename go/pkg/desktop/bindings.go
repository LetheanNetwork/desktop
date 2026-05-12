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
// DialogService — wraps app.Dialog (message, file, folder dialogs).
// ---------------------------------------------------------------------

// DialogFilter describes one file-filter row in an open/save
// dialog. Pattern uses the platform-native shape (Wails normalises
// across OSes) — e.g. "*.gguf;*.safetensors".
type DialogFilter struct {
	Name    string `json:"name"`
	Pattern string `json:"pattern"`
}

// OpenFileOptions describes an open-file dialog. When Multiple is
// true, the binding returns a []string with every selected path.
type OpenFileOptions struct {
	Title     string         `json:"title"`
	Directory string         `json:"directory"`
	Filters   []DialogFilter `json:"filters"`
	Multiple  bool           `json:"multiple"`
}

// SaveFileOptions describes a save-file dialog.
type SaveFileOptions struct {
	Title     string         `json:"title"`
	Filename  string         `json:"filename"`
	Directory string         `json:"directory"`
	Filters   []DialogFilter `json:"filters"`
}

// OpenFolderOptions describes a folder-picker dialog.
type OpenFolderOptions struct {
	Title     string `json:"title"`
	Directory string `json:"directory"`
}

type DialogService struct {
	app *application.App
}

func NewDialogService() *DialogService { return &DialogService{} }

func (s *DialogService) ServiceName() string { return "Dialog" }
func (s *DialogService) ServiceStartup(_ context.Context, _ application.ServiceOptions) error {
	return nil
}
func (s *DialogService) ServiceShutdown() error { return nil }

// Info shows a non-blocking info dialog. Returns when the user
// dismisses it.
func (s *DialogService) Info(title, message string) error {
	if s.app == nil {
		return errors.New("dialog service not yet attached to wails app")
	}
	s.app.Dialog.Info().SetTitle(title).SetMessage(message).Show()
	return nil
}

// Warning shows a warning dialog.
func (s *DialogService) Warning(title, message string) error {
	if s.app == nil {
		return errors.New("dialog service not yet attached to wails app")
	}
	s.app.Dialog.Warning().SetTitle(title).SetMessage(message).Show()
	return nil
}

// ErrorDialog shows an error dialog. Named ErrorDialog (not Error)
// to avoid colliding with Go's error type when the TS binding
// generator inspects the method set.
func (s *DialogService) ErrorDialog(title, message string) error {
	if s.app == nil {
		return errors.New("dialog service not yet attached to wails app")
	}
	s.app.Dialog.Error().SetTitle(title).SetMessage(message).Show()
	return nil
}

// OpenFile shows an open-file picker. Returns the selected path(s) —
// always a slice, even when opts.Multiple is false (length 1 in that
// case). Empty slice = user cancelled.
func (s *DialogService) OpenFile(opts OpenFileOptions) ([]string, error) {
	if s.app == nil {
		return nil, errors.New("dialog service not yet attached to wails app")
	}
	d := s.app.Dialog.OpenFile().SetTitle(opts.Title)
	if opts.Directory != "" {
		d = d.SetDirectory(opts.Directory)
	}
	for _, f := range opts.Filters {
		d = d.AddFilter(f.Name, f.Pattern)
	}
	if opts.Multiple {
		paths, err := d.PromptForMultipleSelection()
		if err != nil {
			return nil, err
		}
		return paths, nil
	}
	path, err := d.PromptForSingleSelection()
	if err != nil {
		return nil, err
	}
	if path == "" {
		return []string{}, nil
	}
	return []string{path}, nil
}

// SaveFile shows a save-file picker. Returns the chosen path, or
// empty string if the user cancelled.
func (s *DialogService) SaveFile(opts SaveFileOptions) (string, error) {
	if s.app == nil {
		return "", errors.New("dialog service not yet attached to wails app")
	}
	d := s.app.Dialog.SaveFile().SetMessage(opts.Title)
	if opts.Filename != "" {
		d = d.SetFilename(opts.Filename)
	}
	if opts.Directory != "" {
		d = d.SetDirectory(opts.Directory)
	}
	for _, f := range opts.Filters {
		d = d.AddFilter(f.Name, f.Pattern)
	}
	return d.PromptForSingleSelection()
}

// OpenFolder shows a folder picker (uses OpenFile with the
// CanChooseDirectories flag, per Wails' API).
func (s *DialogService) OpenFolder(opts OpenFolderOptions) (string, error) {
	if s.app == nil {
		return "", errors.New("dialog service not yet attached to wails app")
	}
	d := s.app.Dialog.OpenFile().
		SetTitle(opts.Title).
		CanChooseFiles(false).
		CanChooseDirectories(true)
	if opts.Directory != "" {
		d = d.SetDirectory(opts.Directory)
	}
	return d.PromptForSingleSelection()
}

// ---------------------------------------------------------------------
// BrowserService — wraps app.Browser (open URL/file in default browser).
// ---------------------------------------------------------------------

type BrowserService struct {
	app *application.App
}

func NewBrowserService() *BrowserService { return &BrowserService{} }

func (s *BrowserService) ServiceName() string { return "Browser" }
func (s *BrowserService) ServiceStartup(_ context.Context, _ application.ServiceOptions) error {
	return nil
}
func (s *BrowserService) ServiceShutdown() error { return nil }

// OpenURL launches the user's default web browser at url. Uses
// macOS `open` / Windows Shell API / Linux xdg-open under the
// hood.
//
// TODO entitlements: route through core.Entitled("network.outbound")
// when permissions are enforced — user might want to gate external
// links via the same `lthn permissions check` surface. For now
// every URL opens.
func (s *BrowserService) OpenURL(url string) error {
	if s.app == nil {
		return errors.New("browser service not yet attached to wails app")
	}
	return s.app.Browser.OpenURL(url)
}

// OpenFile opens a local file in the OS-default handler — browser
// for .html / .pdf / images, system app otherwise. Useful for
// exported chat-history HTML, downloaded model READMEs, generated
// inference reports.
func (s *BrowserService) OpenFile(path string) error {
	if s.app == nil {
		return errors.New("browser service not yet attached to wails app")
	}
	return s.app.Browser.OpenFile(path)
}

// ---------------------------------------------------------------------
// WindowService — open / hide / focus named windows from the frontend.
// ---------------------------------------------------------------------

type WindowService struct {
	app *application.App
}

func NewWindowService() *WindowService { return &WindowService{} }

func (s *WindowService) ServiceName() string { return "Window" }
func (s *WindowService) ServiceStartup(_ context.Context, _ application.ServiceOptions) error {
	return nil
}
func (s *WindowService) ServiceShutdown() error { return nil }

// Open shows + focuses the named window (chat / models / settings /
// about). No-op if the name isn't in the windows.go registry.
func (s *WindowService) Open(name string) error {
	if s.app == nil {
		return errors.New("window service not yet attached to wails app")
	}
	openWindow(s.app, name)
	return nil
}

// Hide hides the named window. Steady-state windows (chat / models
// / settings) hide-on-close anyway; this lets the frontend dismiss
// programmatically without waiting for a close click.
func (s *WindowService) Hide(name string) error {
	if s.app == nil {
		return errors.New("window service not yet attached to wails app")
	}
	w, ok := s.app.Window.GetByName(name)
	if !ok {
		return errors.New("no window named: " + name)
	}
	w.Hide()
	return nil
}

// List returns the names of every registered window. Frontend can
// render a "window switcher" or jump-list from this.
func (s *WindowService) List() ([]string, error) {
	registry := windowRegistry()
	names := make([]string, len(registry))
	for i, spec := range registry {
		names[i] = spec.Name
	}
	return names, nil
}

// ---------------------------------------------------------------------
// ScreenService — wraps app.Screen (multi-monitor info).
// ---------------------------------------------------------------------

// ScreenInfo is the JSON-stable shape exposed to the frontend.
// Mirrors application.Screen but uses flat width/height instead
// of the nested Size struct so the TS bindings are easier to read.
type ScreenInfo struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	X           int     `json:"x"`
	Y           int     `json:"y"`
	Width       int     `json:"width"`
	Height      int     `json:"height"`
	ScaleFactor float32 `json:"scale_factor"`
	IsPrimary   bool    `json:"is_primary"`
	Rotation    float32 `json:"rotation"`
}

func screenToInfo(s *application.Screen) ScreenInfo {
	if s == nil {
		return ScreenInfo{}
	}
	return ScreenInfo{
		ID:          s.ID,
		Name:        s.Name,
		X:           s.X,
		Y:           s.Y,
		Width:       s.Size.Width,
		Height:      s.Size.Height,
		ScaleFactor: s.ScaleFactor,
		IsPrimary:   s.IsPrimary,
		Rotation:    s.Rotation,
	}
}

type ScreenService struct {
	// app is set by desktop.Run() post-construction.
	app *application.App
}

func NewScreenService() *ScreenService { return &ScreenService{} }

func (s *ScreenService) ServiceName() string { return "Screen" }
func (s *ScreenService) ServiceStartup(_ context.Context, _ application.ServiceOptions) error {
	return nil
}
func (s *ScreenService) ServiceShutdown() error { return nil }

// All returns every connected display.
//
// Use case: model-load dialog asks the user which monitor to
// anchor the chat window to; settings page renders a layout
// preview from the bounds.
func (s *ScreenService) All() ([]ScreenInfo, error) {
	if s.app == nil {
		return nil, errors.New("screen service not yet attached to wails app")
	}
	raw := s.app.Screen.GetAll()
	out := make([]ScreenInfo, 0, len(raw))
	for _, r := range raw {
		out = append(out, screenToInfo(r))
	}
	return out, nil
}

// Primary returns the OS-designated primary display — the one with
// the menubar on macOS, the taskbar's anchor on Windows.
func (s *ScreenService) Primary() (ScreenInfo, error) {
	if s.app == nil {
		return ScreenInfo{}, errors.New("screen service not yet attached to wails app")
	}
	return screenToInfo(s.app.Screen.GetPrimary()), nil
}

// ByID returns the screen with the given ID, or an error if no
// such display exists. ID strings vary by OS — use All() to
// discover them.
func (s *ScreenService) ByID(id string) (ScreenInfo, error) {
	if s.app == nil {
		return ScreenInfo{}, errors.New("screen service not yet attached to wails app")
	}
	scr := s.app.Screen.GetByID(id)
	if scr == nil {
		return ScreenInfo{}, errors.New("no screen with id: " + id)
	}
	return screenToInfo(scr), nil
}

// ---------------------------------------------------------------------
// ClipboardService — wraps app.Clipboard (text copy/paste).
// ---------------------------------------------------------------------

type ClipboardService struct {
	// app is set by desktop.Run() AFTER application.New returns —
	// app.Clipboard isn't available pre-construction.
	app *application.App
}

func NewClipboardService() *ClipboardService { return &ClipboardService{} }

func (s *ClipboardService) ServiceName() string { return "Clipboard" }
func (s *ClipboardService) ServiceStartup(_ context.Context, _ application.ServiceOptions) error {
	return nil
}
func (s *ClipboardService) ServiceShutdown() error { return nil }

// Copy writes text to the system clipboard.
//
// Why expose this when the WebView already has navigator.clipboard?
// The browser clipboard API requires user-gesture context and only
// works under HTTPS/localhost — fine for most paths but breaks for
// background-triggered copies (e.g. "copy session export to
// clipboard" from a notification action). The Wails surface has
// no gesture/origin gate.
func (s *ClipboardService) Copy(text string) error {
	if s.app == nil {
		return errors.New("clipboard service not yet attached to wails app")
	}
	if !s.app.Clipboard.SetText(text) {
		return errors.New("clipboard SetText returned false")
	}
	return nil
}

// Paste reads text from the system clipboard.
func (s *ClipboardService) Paste() (string, error) {
	if s.app == nil {
		return "", errors.New("clipboard service not yet attached to wails app")
	}
	text, ok := s.app.Clipboard.Text()
	if !ok {
		return "", errors.New("clipboard empty or unreadable")
	}
	return text, nil
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
