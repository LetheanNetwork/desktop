// SPDX-Licence-Identifier: EUPL-1.2

// Package ml owns lthn/desktop's persistent state for the ML subsystem.
// Backs ~/Lethean/data/desktop/ml.duckdb — separate from the master DB
// so ML workloads (training run records, LoRA / GRPO job state, dataset
// manifests, evaluation results, model-pack registry) can grow without
// pressuring the fleet/tasks master lock. Companion to pkg/ai.
//
// The Service holds a DuckDB handle that callers reach via DB(). Schema
// growth is feature-driven — additions land alongside the consumer
// (e.g. distillation-window records training runs here when training
// orchestration ships).
//
// Usage example:
//
//	core.New(core.WithName("ml", ml.Register))
package ml

import (
	core "dappco.re/go"
	"dappco.re/go/store"
	"dappco.re/lthn/desktop/pkg/paths"
)

// Service is the lthn-side owner of the ml.duckdb store. db is nil
// when the file can't be opened (another lthn instance holds the
// write lock) — methods check before issuing queries, matching the
// pkg/fleet degraded-Service contract.
type Service struct {
	db *store.DuckDB
}

// New opens ~/Lethean/data/desktop/ml.duckdb in read-write mode.
// Creates the parent directory on first call. Returns Result with
// Value = *Service on OK; the caller (Register / consumers) decides
// whether a degraded handle is acceptable on lock conflict.
//
// Usage example:
//
//	r := ml.New()
//	if r.OK { svc := r.Value.(*ml.Service); _ = svc }
func New() core.Result {
	dbPathR := paths.MLDB()
	if !dbPathR.OK {
		return dbPathR
	}
	if r := paths.DesktopDir(); !r.OK {
		return r
	}
	db, openR := store.OpenDuckDBReadWrite(dbPathR.Value.(string))
	if !openR.OK {
		return core.Fail(core.E("ml.New", "open ml DuckDB", openR.Value.(error)))
	}
	return core.Ok(&Service{db: db})
}

// Register is the core.WithName-compatible factory. Lock-tolerant per
// the pkg/fleet pattern: when another lthn process holds the
// ml.duckdb lock, register a degraded Service so Core boot continues.
//
// Usage example:
//
//	core.New(core.WithName("ml", ml.Register))
func Register(c *core.Core) core.Result {
	r := New()
	if r.OK {
		return r
	}
	core.Warn("ml.Register: ml.duckdb unavailable, registering degraded Service", "err", r.Error())
	return core.Ok(&Service{})
}

// DB returns the underlying DuckDB handle. nil when the Service is
// degraded (lock conflict at registration). Callers should check
// before issuing queries; the receiver matches pkg/fleet's idiom.
//
// Usage example:
//
//	db := svc.DB()
//	if db == nil { return core.Fail(core.NewError("ml: service closed")) }
func (s *Service) DB() *store.DuckDB { return s.db }

// Close releases the underlying handle. Idempotent.
//
// Usage example:
//
//	r := svc.Close()
func (s *Service) Close() core.Result {
	if s.db == nil {
		return core.Ok(nil)
	}
	r := s.db.Close()
	s.db = nil
	return r
}
