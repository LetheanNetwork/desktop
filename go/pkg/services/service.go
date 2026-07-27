// SPDX-Licence-Identifier: EUPL-1.2

package services

import (
	"runtime"
	"sort"
	"sync"
	"time"

	core "dappco.re/go"
	coreio "dappco.re/go/io"
	coreprocess "dappco.re/go/process"
	"dappco.re/lthn/desktop/pkg/audit"
)

const defaultCataloguePath = "desktop/services/catalogue.json"

// Options configures the managed-service catalogue and process boundary.
type Options struct {
	Process                  ProcessRuntime
	Catalogue                Catalogue
	Builtins                 []Definition
	Limits                   Limits
	WorkingDirectoryResolver WorkingDirectoryResolver
	Now                      func() time.Time
	After                    func(time.Duration) <-chan time.Time
	Audit                    audit.Recorder
}

type runtimeRecord struct {
	definition    Definition
	snapshot      Snapshot
	process       ProcessHandle
	generation    uint64
	operation     bool
	restartCancel core.CancelFunc
	restartTimes  []time.Time
}

// Service is the manual-by-default owner of optional background capability
// definitions and their go-process lifecycle.
type Service struct {
	*core.ServiceRuntime[Options]

	options Options
	ctx     core.Context

	mu             sync.RWMutex
	catalogueMu    sync.Mutex
	document       CatalogueDocument
	records        map[string]*runtimeRecord
	builtinIDs     map[string]bool
	started        bool
	available      bool
	startupFailure *Failure
	shuttingDown   bool
}

// NewService constructs a manager without loading or starting anything.
func NewService(options Options) *Service {
	options.Limits = normaliseServiceLimits(options.Limits)
	options.Builtins = cloneDefinitions(options.Builtins)
	if options.WorkingDirectoryResolver == nil {
		options.WorkingDirectoryResolver = emptyWorkingDirectoryResolver{}
	}
	if options.Now == nil {
		options.Now = core.Now
	}
	if options.After == nil {
		options.After = time.After
	}
	if options.Audit == nil {
		options.Audit = defaultAuditRecorder{}
	}
	return &Service{
		options:    options,
		records:    make(map[string]*runtimeRecord),
		builtinIDs: make(map[string]bool),
	}
}

// Register resolves the named go-process and go-io services, then registers
// the canonical Desktop managed-service manager.
//
// Usage example:
//
//	c := core.New(
//		core.WithName("process", process.NewService(process.Options{})),
//		core.WithName("io", io.NewService(io.IOConfig{})),
//		core.WithName("services", services.Register),
//	)
func Register(c *core.Core) core.Result {
	if c == nil {
		return failureResult(
			ErrorServicesUnavailable,
			"services.Register",
			"Core is unavailable.",
			nil,
		)
	}
	processService, ok := core.ServiceFor[*coreprocess.Service](c, "process")
	if !ok || processService == nil {
		return failureResult(
			ErrorServicesUnavailable,
			"services.Register",
			"The named process service is unavailable.",
			nil,
		)
	}
	ioService, ok := core.ServiceFor[*coreio.Service](c, "io")
	if !ok || ioService == nil || ioService.Medium == nil {
		return failureResult(
			ErrorServicesUnavailable,
			"services.Register",
			"The named I/O Medium is unavailable.",
			nil,
		)
	}
	args := core.Args()
	if len(args) == 0 || args[0] == "" {
		return failureResult(
			ErrorServicesUnavailable,
			"services.Register",
			"The Lethean executable is unavailable.",
			nil,
		)
	}
	limits := DefaultLimits()
	manager := NewService(Options{
		Process: &namedProcessRuntime{service: processService},
		Catalogue: NewMediumCatalogue(
			ioService.Medium,
			defaultCataloguePath,
			limits,
		),
		Builtins: []Definition{
			{
				ID:                "serve",
				DisplayName:       "Lethean Desktop API",
				Description:       "OpenAI-compatible local Lethean API.",
				Kind:              KindService,
				Command:           args[0],
				Arguments:         []string{"serve"},
				RestartPolicy:     RestartNever,
				GracePeriodMillis: 5_000,
				Owner:             "lethean",
			},
			{
				ID:          "inference",
				DisplayName: "LEM inference runtime",
				Description: "Optional local model runtime, started only on request.",
				Kind:        KindService,
				Command:     inferenceExecutable(args[0]),
				Arguments: []string{
					"serve",
					"--addr",
					"127.0.0.1:36911",
					"--shutdown-timeout",
					"10s",
				},
				RestartPolicy:     RestartNever,
				GracePeriodMillis: 15_000,
				Owner:             "lethean",
			},
		},
		Limits: limits,
	})
	return manager.Register(c)
}

func inferenceExecutable(lthnExecutable string) string {
	name := "lem"
	if runtime.GOOS == "windows" {
		name = "lem.exe"
	}
	return core.PathJoin(core.PathDir(lthnExecutable), name)
}

// Register attaches this instance to Core and returns it as the named service.
//
// Usage example:
//
//	manager := services.NewService(options)
//	core.WithName("services", manager.Register)
func (service *Service) Register(c *core.Core) core.Result {
	if service == nil || c == nil {
		return failureResult(
			ErrorServicesUnavailable,
			"services.Service.Register",
			"Services manager registration is unavailable.",
			nil,
		)
	}
	service.mu.Lock()
	service.ServiceRuntime = core.NewServiceRuntime(c, service.options)
	service.mu.Unlock()
	return core.Ok(service)
}

// OnStartup loads and validates definitions without starting a process.
func (service *Service) OnStartup(ctx core.Context) core.Result {
	if service == nil {
		return core.Ok(nil)
	}
	service.mu.Lock()
	if service.started {
		service.mu.Unlock()
		return core.Ok(nil)
	}
	service.started = true
	if ctx == nil {
		ctx = core.Background()
	}
	service.ctx = ctx
	service.mu.Unlock()

	if service.options.Process == nil {
		service.setUnavailable(failureResult(
			ErrorServicesUnavailable,
			"services.Service.OnStartup",
			"The named process service is unavailable.",
			nil,
		))
		return core.Ok(nil)
	}
	if service.options.Catalogue == nil {
		service.setUnavailable(failureResult(
			ErrorServicesUnavailable,
			"services.Service.OnStartup",
			"Services catalogue storage is unavailable.",
			nil,
		))
		return core.Ok(nil)
	}
	loaded := service.options.Catalogue.Load()
	if !loaded.OK {
		service.setUnavailable(loaded)
		return core.Ok(nil)
	}
	document, ok := loaded.Value.(CatalogueDocument)
	if !ok {
		service.setUnavailable(failureResult(
			ErrorCatalogueInvalid,
			"services.Service.OnStartup",
			"Services catalogue returned an invalid document.",
			nil,
		))
		return core.Ok(nil)
	}
	records, builtinIDs, result := service.composeRecords(document)
	if !result.OK {
		service.setUnavailable(result)
		return core.Ok(nil)
	}

	service.mu.Lock()
	service.document = cloneCatalogueDocument(document)
	service.records = records
	service.builtinIDs = builtinIDs
	service.available = true
	service.startupFailure = nil
	service.mu.Unlock()
	return core.Ok(nil)
}

// Catalogue returns immutable renderer-safe snapshots without execution data.
func (service *Service) Catalogue() core.Result {
	service.mu.RLock()
	defer service.mu.RUnlock()
	if result := service.readyLocked("services.Service.Catalogue"); !result.OK {
		return result
	}
	ids := make([]string, 0, len(service.records))
	for id := range service.records {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	snapshots := make([]Snapshot, 0, len(ids))
	for _, id := range ids {
		snapshots = append(snapshots, cloneSnapshot(service.records[id].snapshot))
	}
	return core.Ok(CatalogueView{
		Services:    snapshots,
		RefreshedAt: service.options.Now().UTC().Format(time.RFC3339Nano),
	})
}

// Get returns an immutable snapshot for a known trusted definition ID.
func (service *Service) Get(id string) core.Result {
	service.mu.RLock()
	defer service.mu.RUnlock()
	if result := service.readyLocked("services.Service.Get"); !result.OK {
		return result
	}
	record, ok := service.records[id]
	if !ok {
		return definitionNotFound("services.Service.Get", id)
	}
	return core.Ok(cloneSnapshot(record.snapshot))
}

// EnsureDefinition validates and durably publishes a trusted Go definition.
func (service *Service) EnsureDefinition(definition Definition) core.Result {
	if result := ValidateDefinition(definition, service.options.Limits); !result.OK {
		return result
	}
	service.catalogueMu.Lock()
	defer service.catalogueMu.Unlock()

	service.mu.Lock()
	if result := service.readyLocked("services.Service.EnsureDefinition"); !result.OK {
		service.mu.Unlock()
		return result
	}
	existing, exists := service.records[definition.ID]
	if exists {
		if existing.definition.Owner != definition.Owner {
			service.mu.Unlock()
			return failureResult(
				ErrorDefinitionConflict,
				"services.Service.EnsureDefinition",
				"Another package owns this service definition.",
				nil,
			)
		}
		if recordBusy(existing) {
			service.mu.Unlock()
			return operationInProgress("services.Service.EnsureDefinition", definition.ID)
		}
		existing.operation = true
	}
	document := cloneCatalogueDocument(service.document)
	upsertDefinition(&document, definition)
	document.UpdatedAt = service.options.Now().UTC().Format(time.RFC3339Nano)
	service.mu.Unlock()

	saved := service.options.Catalogue.Save(document)

	service.mu.Lock()
	if exists {
		existing.operation = false
	}
	if !saved.OK {
		service.mu.Unlock()
		return saved
	}
	applied := applyPolicyOverride(definition, document.PolicyOverrides)
	snapshot := Snapshot{
		Definition: definitionView(applied),
		State:      StateStopped,
	}
	previous := snapshot
	if exists {
		previous = cloneSnapshot(existing.snapshot)
		existing.definition = cloneDefinition(applied)
		existing.snapshot = snapshot
		existing.process = nil
	} else {
		service.records[definition.ID] = &runtimeRecord{
			definition: cloneDefinition(applied),
			snapshot:   snapshot,
		}
	}
	service.document = cloneCatalogueDocument(document)
	service.mu.Unlock()
	service.fireEvent("definition", previous, snapshot, "")
	service.auditDefinitionChanged(definition.ID)
	return core.Ok(cloneSnapshot(snapshot))
}

// RemoveDefinition durably removes a trusted package-owned definition.
func (service *Service) RemoveDefinition(owner, id string) core.Result {
	service.catalogueMu.Lock()
	defer service.catalogueMu.Unlock()

	service.mu.Lock()
	if result := service.readyLocked("services.Service.RemoveDefinition"); !result.OK {
		service.mu.Unlock()
		return result
	}
	record, ok := service.records[id]
	if !ok {
		service.mu.Unlock()
		return definitionNotFound("services.Service.RemoveDefinition", id)
	}
	if service.builtinIDs[id] || record.definition.Owner != owner {
		service.mu.Unlock()
		return failureResult(
			ErrorDefinitionConflict,
			"services.Service.RemoveDefinition",
			"This package does not own the service definition.",
			nil,
		)
	}
	if recordBusy(record) {
		service.mu.Unlock()
		return operationInProgress("services.Service.RemoveDefinition", id)
	}
	record.operation = true
	document := cloneCatalogueDocument(service.document)
	removeDefinitionFromDocument(&document, id)
	document.UpdatedAt = service.options.Now().UTC().Format(time.RFC3339Nano)
	service.mu.Unlock()

	saved := service.options.Catalogue.Save(document)

	service.mu.Lock()
	record.operation = false
	if !saved.OK {
		service.mu.Unlock()
		return saved
	}
	previous := cloneSnapshot(record.snapshot)
	delete(service.records, id)
	service.document = cloneCatalogueDocument(document)
	service.mu.Unlock()
	next := previous
	next.State = StateStopped
	next.Desired = false
	next.ProcessID = ""
	next.PID = 0
	service.fireEvent("definition", previous, next, "")
	service.auditDefinitionChanged(id)
	return core.Ok(nil)
}

// SetPolicy durably updates bounded restart policy for a known definition.
func (service *Service) SetPolicy(override PolicyOverride) core.Result {
	if !definitionIDPattern.MatchString(override.ID) ||
		!validRestartPolicy(override.RestartPolicy) ||
		override.GracePeriodMillis <= 0 ||
		override.GracePeriodMillis > service.options.Limits.MaxGracePeriodMillis {
		return failureResult(
			ErrorDefinitionInvalid,
			"services.Service.SetPolicy",
			"Service policy is invalid.",
			nil,
		)
	}
	service.catalogueMu.Lock()
	defer service.catalogueMu.Unlock()

	service.mu.Lock()
	if result := service.readyLocked("services.Service.SetPolicy"); !result.OK {
		service.mu.Unlock()
		return result
	}
	record, ok := service.records[override.ID]
	if !ok {
		service.mu.Unlock()
		return definitionNotFound("services.Service.SetPolicy", override.ID)
	}
	if recordBusy(record) {
		service.mu.Unlock()
		return operationInProgress("services.Service.SetPolicy", override.ID)
	}
	record.operation = true
	document := cloneCatalogueDocument(service.document)
	upsertPolicyOverride(&document, override)
	document.UpdatedAt = service.options.Now().UTC().Format(time.RFC3339Nano)
	service.mu.Unlock()

	saved := service.options.Catalogue.Save(document)

	service.mu.Lock()
	record.operation = false
	if !saved.OK {
		service.mu.Unlock()
		return saved
	}
	previous := cloneSnapshot(record.snapshot)
	record.definition.RestartPolicy = override.RestartPolicy
	record.definition.GracePeriodMillis = override.GracePeriodMillis
	record.snapshot.Definition = definitionView(record.definition)
	service.document = cloneCatalogueDocument(document)
	next := cloneSnapshot(record.snapshot)
	service.mu.Unlock()
	service.fireEvent("definition", previous, next, "")
	service.auditDefinitionChanged(override.ID)
	return core.Ok(next)
}

func (service *Service) composeRecords(
	document CatalogueDocument,
) (map[string]*runtimeRecord, map[string]bool, core.Result) {
	definitions := make(map[string]Definition)
	builtinIDs := make(map[string]bool)
	for _, definition := range service.options.Builtins {
		if result := ValidateDefinition(definition, service.options.Limits); !result.OK {
			return nil, nil, failureResult(
				ErrorDefinitionInvalid,
				"services.Service.OnStartup",
				"A built-in service definition is invalid.",
				result.Err(),
			)
		}
		if _, exists := definitions[definition.ID]; exists {
			return nil, nil, failureResult(
				ErrorDefinitionConflict,
				"services.Service.OnStartup",
				"Built-in service definitions contain a duplicate ID.",
				nil,
			)
		}
		definitions[definition.ID] = cloneDefinition(definition)
		builtinIDs[definition.ID] = true
	}
	for _, definition := range document.Definitions {
		if existing, exists := definitions[definition.ID]; exists &&
			existing.Owner != definition.Owner {
			return nil, nil, failureResult(
				ErrorDefinitionConflict,
				"services.Service.OnStartup",
				"Persisted service ownership conflicts with a built-in definition.",
				nil,
			)
		}
		definitions[definition.ID] = cloneDefinition(definition)
	}
	records := make(map[string]*runtimeRecord, len(definitions))
	for id, definition := range definitions {
		definition = applyPolicyOverride(definition, document.PolicyOverrides)
		records[id] = &runtimeRecord{
			definition: cloneDefinition(definition),
			snapshot: Snapshot{
				Definition: definitionView(definition),
				State:      StateStopped,
			},
		}
	}
	return records, builtinIDs, core.Ok(nil)
}

func (service *Service) setUnavailable(result core.Result) {
	failure := failureFromResult(
		result,
		ErrorServicesUnavailable,
		"services.Service",
		"Services manager is unavailable.",
	)
	service.mu.Lock()
	service.available = false
	service.startupFailure = failure
	service.mu.Unlock()
}

func (service *Service) readyLocked(operation string) core.Result {
	if service == nil || !service.started || !service.available {
		if service != nil && service.startupFailure != nil {
			return core.Fail(cloneFailure(service.startupFailure))
		}
		return failureResult(
			ErrorServicesUnavailable,
			operation,
			"Services manager is unavailable.",
			nil,
		)
	}
	if service.shuttingDown {
		return failureResult(
			ErrorServicesUnavailable,
			operation,
			"Services manager is shutting down.",
			nil,
		)
	}
	return core.Ok(nil)
}

func normaliseServiceLimits(limits Limits) Limits {
	defaults := DefaultLimits()
	if limits.MaxDefinitions <= 0 {
		limits.MaxDefinitions = defaults.MaxDefinitions
	}
	if limits.MaxArguments <= 0 {
		limits.MaxArguments = defaults.MaxArguments
	}
	if limits.MaxArgumentBytes <= 0 {
		limits.MaxArgumentBytes = defaults.MaxArgumentBytes
	}
	if limits.MaxRunning <= 0 {
		limits.MaxRunning = defaults.MaxRunning
	}
	if limits.MaxOutputBytes <= 0 {
		limits.MaxOutputBytes = defaults.MaxOutputBytes
	}
	if limits.MaxGracePeriodMillis <= 0 {
		limits.MaxGracePeriodMillis = defaults.MaxGracePeriodMillis
	}
	if limits.RestartLimit <= 0 {
		limits.RestartLimit = defaults.RestartLimit
	}
	if limits.RestartWindow <= 0 {
		limits.RestartWindow = defaults.RestartWindow
	}
	if limits.RestartBaseDelay <= 0 {
		limits.RestartBaseDelay = defaults.RestartBaseDelay
	}
	if limits.RestartMaxDelay <= 0 {
		limits.RestartMaxDelay = defaults.RestartMaxDelay
	}
	if limits.RestartExponentCap <= 0 {
		limits.RestartExponentCap = defaults.RestartExponentCap
	}
	return limits
}

func cloneDefinitions(definitions []Definition) []Definition {
	clones := make([]Definition, len(definitions))
	for index, definition := range definitions {
		clones[index] = cloneDefinition(definition)
	}
	return clones
}

func recordBusy(record *runtimeRecord) bool {
	return record == nil ||
		record.operation ||
		record.process != nil ||
		record.snapshot.State == StateStarting ||
		record.snapshot.State == StateRunning ||
		record.snapshot.State == StateStopping
}

func applyPolicyOverride(
	definition Definition,
	overrides []PolicyOverride,
) Definition {
	for _, override := range overrides {
		if override.ID == definition.ID {
			definition.RestartPolicy = override.RestartPolicy
			definition.GracePeriodMillis = override.GracePeriodMillis
			break
		}
	}
	return definition
}

func upsertDefinition(document *CatalogueDocument, definition Definition) {
	for index := range document.Definitions {
		if document.Definitions[index].ID == definition.ID {
			document.Definitions[index] = cloneDefinition(definition)
			return
		}
	}
	document.Definitions = append(document.Definitions, cloneDefinition(definition))
	sort.Slice(document.Definitions, func(left, right int) bool {
		return document.Definitions[left].ID < document.Definitions[right].ID
	})
}

func removeDefinitionFromDocument(document *CatalogueDocument, id string) {
	definitions := document.Definitions[:0]
	for _, definition := range document.Definitions {
		if definition.ID != id {
			definitions = append(definitions, definition)
		}
	}
	document.Definitions = definitions
	overrides := document.PolicyOverrides[:0]
	for _, override := range document.PolicyOverrides {
		if override.ID != id {
			overrides = append(overrides, override)
		}
	}
	document.PolicyOverrides = overrides
}

func upsertPolicyOverride(document *CatalogueDocument, override PolicyOverride) {
	for index := range document.PolicyOverrides {
		if document.PolicyOverrides[index].ID == override.ID {
			document.PolicyOverrides[index] = override
			return
		}
	}
	document.PolicyOverrides = append(document.PolicyOverrides, override)
	sort.Slice(document.PolicyOverrides, func(left, right int) bool {
		return document.PolicyOverrides[left].ID < document.PolicyOverrides[right].ID
	})
}

func definitionNotFound(operation, id string) core.Result {
	return failureResult(
		ErrorDefinitionNotFound,
		operation,
		core.Concat("Unknown service ID: ", id, "."),
		nil,
	)
}

func operationInProgress(operation, id string) core.Result {
	return failureResult(
		ErrorOperationInProgress,
		operation,
		core.Concat("A service operation is already in progress for ", id, "."),
		nil,
	)
}

func failureResult(
	code ErrorCode,
	operation string,
	message string,
	cause error,
) core.Result {
	return core.Fail(&Failure{
		Code:      code,
		Operation: operation,
		Message:   message,
		Cause:     cause,
	})
}

func failureFromResult(
	result core.Result,
	fallbackCode ErrorCode,
	operation string,
	message string,
) *Failure {
	var failure *Failure
	if !result.OK && core.As(result.Err(), &failure) && failure != nil {
		return cloneFailure(failure)
	}
	return &Failure{
		Code:      fallbackCode,
		Operation: operation,
		Message:   message,
		Cause:     result.Err(),
	}
}

func cloneFailure(failure *Failure) *Failure {
	if failure == nil {
		return nil
	}
	clone := *failure
	return &clone
}
