// SPDX-License-Identifier: EUPL-1.2

// Package files exposes the canonical provider-neutral Files application.
// Every observation and mutation is addressed by a registered mount ID and a
// relative path, then performed through that mount's io.Medium.
package files

import (
	goio "io"
	"sync"

	core "dappco.re/go"
	coreio "dappco.re/go/io"
)

// Service owns the canonical provider-neutral Files surface.
//
// Usage example:
//
//	svc := files.NewService(files.Options{Core: c, Runtime: runtime})
type Service struct {
	core          *core.Core
	pendingMounts []Mount
	mounts        map[string]Mount
	runtime       RuntimeMetadata
	runtimeMu     sync.Mutex
	limits        Limits
	locks         map[string]*sync.Mutex
	internalReady map[string]bool
	hostMu        sync.RWMutex
	hostMounts    map[string]Mount
	hostOrder     []string

	hostMediumFactory func(string) (coreio.Medium, error)
	shutdownOnce      sync.Once
	shutdownErr       error
}

// NewService constructs the Files service from trusted mount composition.
//
// Usage example:
//
//	svc := files.NewService(files.Options{Core: c, Runtime: runtime})
func NewService(options Options) *Service {
	limits := options.Limits
	if limits == (Limits{}) {
		limits = DefaultLimits()
	}
	return &Service{
		core:          options.Core,
		pendingMounts: append([]Mount(nil), options.Mounts...),
		mounts:        make(map[string]Mount),
		runtime:       options.Runtime,
		limits:        limits,
		locks:         make(map[string]*sync.Mutex),
		internalReady: make(map[string]bool),
		hostMounts:    make(map[string]Mount),
		hostMediumFactory: func(root string) (coreio.Medium, error) {
			return coreio.NewSandboxed(root)
		},
	}
}

// Register constructs the Files service for Core registration.
//
// Usage example:
//
//	core.New(core.WithName("office-files", files.Register))
func Register(c *core.Core) core.Result {
	optionsResult := DefaultOptions(c)
	if !optionsResult.OK {
		return optionsResult
	}
	options, ok := optionsResult.Value.(Options)
	if !ok {
		return core.Fail(newFailure(
			ErrorInvalidInput,
			"",
			"",
			"default Files options are invalid",
			nil,
		))
	}
	return NewService(options).Register(c)
}

// Register validates trusted mount composition before exposing the service.
func (s *Service) Register(c *core.Core) core.Result {
	if s == nil || s.runtime == nil {
		return core.Fail(newFailure(
			ErrorInvalidInput,
			"",
			"",
			"runtime Medium is required",
			nil,
		))
	}
	s.core = c
	for _, mount := range s.pendingMounts {
		if err := s.addMount(mount); err != nil {
			return core.Fail(err)
		}
		s.initialiseInternalNamespace(mount)
	}
	return core.Ok(s)
}

func (s *Service) addMount(mount Mount) error {
	if err := validateMountID(mount.ID); err != nil {
		return err
	}
	if _, exists := s.mounts[mount.ID]; exists {
		return newFailure(
			ErrorInvalidMount,
			mount.ID,
			"",
			"mount ID is already registered",
			nil,
		)
	}
	if mount.Name == "" || mount.Medium == nil {
		return newFailure(
			ErrorInvalidMount,
			mount.ID,
			"",
			"mount name and Medium are required",
			nil,
		)
	}
	if mount.LocalRoot != "" &&
		(mount.Kind != "local" || !core.PathIsAbs(mount.LocalRoot)) {
		return newFailure(
			ErrorBoundaryRejected,
			mount.ID,
			"",
			"local mount root is not an audited absolute local root",
			nil,
		)
	}
	if !mount.ContainmentAudited {
		return newFailure(
			ErrorBoundaryRejected,
			mount.ID,
			"",
			"mount containment has not been audited",
			nil,
		)
	}
	s.mounts[mount.ID] = mount
	s.locks[mount.ID] = &sync.Mutex{}
	s.internalReady[mount.ID] = false
	return nil
}

func (s *Service) mount(id string) (Mount, error) {
	if err := validateMountID(id); err != nil {
		return Mount{}, err
	}
	mount, ok := s.mounts[id]
	if !ok {
		s.hostMu.RLock()
		mount, ok = s.hostMounts[id]
		s.hostMu.RUnlock()
	}
	if !ok {
		return Mount{}, newFailure(
			ErrorInvalidMount,
			id,
			"",
			"mount is not registered",
			nil,
		)
	}
	return mount, nil
}

// ServiceName labels the sole Wails Files binding.
func (s *Service) ServiceName() string { return "Files" }

// OnShutdown releases only media whose lifecycle was transferred to this
// service. The application io service that stores runtime metadata remains
// owned by Core and is closed by its own lifecycle.
func (s *Service) OnShutdown(core.Context) core.Result {
	if s == nil {
		return core.Ok(nil)
	}
	s.shutdownOnce.Do(func() {
		mounts := make([]Mount, 0, len(s.mounts))
		for _, mount := range s.mounts {
			mounts = append(mounts, mount)
		}
		mounts = append(mounts, s.hostMountSnapshot()...)
		for _, mount := range mounts {
			if !mount.Owned {
				continue
			}
			closer, ok := mount.Medium.(goio.Closer)
			if !ok {
				continue
			}
			if err := closer.Close(); err != nil && s.shutdownErr == nil {
				s.shutdownErr = core.E(
					"files.Service.OnShutdown",
					core.Concat("close mount ", mount.ID),
					err,
				)
			}
		}
	})
	if s.shutdownErr != nil {
		return core.Fail(s.shutdownErr)
	}
	return core.Ok(nil)
}
