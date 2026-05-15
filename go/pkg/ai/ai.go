// SPDX-Licence-Identifier: EUPL-1.2

// Package ai owns lthn/desktop's persistent state for the AI subsystem.
// Backs ~/Lethean/data/desktop/ai.duckdb — separate from the master DB
// so AI workloads (long chats, tool transcripts, provider-route tables,
// training-data candidates) can grow without contending for the
// fleet/tasks master lock. Companion to pkg/ml.
//
// The Service holds a DuckDB handle that callers reach via DB(). Schema
// growth is feature-driven — additions land alongside the consumer
// (e.g. pkg/runner records routes here when route persistence ships).
//
// Usage example:
//
//	core.New(core.WithName("ai", ai.Register))
package ai

import (
	core "dappco.re/go"
	"dappco.re/go/store"
	"dappco.re/lthn/desktop/pkg/paths"
)

// Service is the lthn-side owner of the ai.duckdb store. db is nil
// when the file can't be opened (another lthn instance holds the
// write lock) — methods check before issuing queries, matching the
// pkg/fleet degraded-Service contract.
type Service struct {
	db *store.DuckDB
}

// New opens ~/Lethean/data/desktop/ai.duckdb in read-write mode.
// Creates the parent directory on first call. Returns Result with
// Value = *Service on OK; the caller (Register / consumers) decides
// whether a degraded handle is acceptable on lock conflict.
//
// Usage example:
//
//	r := ai.New()
//	if r.OK { svc := r.Value.(*ai.Service); _ = svc }
func New() core.Result {
	dbPathR := paths.AIDB()
	if !dbPathR.OK {
		return dbPathR
	}
	if r := paths.DesktopDir(); !r.OK {
		return r
	}
	db, openR := store.OpenDuckDBReadWrite(dbPathR.Value.(string))
	if !openR.OK {
		return core.Fail(core.E("ai.New", "open ai DuckDB", openR.Value.(error)))
	}
	return core.Ok(&Service{db: db})
}

// Register is the core.WithName-compatible factory. Lock-tolerant per
// the pkg/fleet pattern: when another lthn process holds the
// ai.duckdb lock, register a degraded Service so Core boot continues.
// Read paths can still work against a /tmp snapshot if a future
// inspector needs them; write paths surface "service closed" via the
// nil-db guards on consumer methods.
//
// Usage example:
//
//	core.New(core.WithName("ai", ai.Register))
func Register(c *core.Core) core.Result {
	r := New()
	if r.OK {
		return r
	}
	core.Warn("ai.Register: ai.duckdb unavailable, registering degraded Service", "err", r.Error())
	return core.Ok(&Service{})
}

// DB returns the underlying DuckDB handle. nil when the Service is
// degraded (lock conflict at registration). Callers should check
// before issuing queries; the receiver matches pkg/fleet's idiom.
//
// Usage example:
//
//	db := svc.DB()
//	if db == nil { return core.Fail(core.NewError("ai: service closed")) }
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
