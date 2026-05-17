// SPDX-Licence-Identifier: EUPL-1.2

// chain_test.go — Mantis #1651 MED N4 substrate-hardening tests:
//
//   - TestAudit_ConcurrentWrite_NoCorruption_Good — 16 goroutines firing
//     ~50 events each; the resulting file MUST contain exactly the
//     expected number of NDJSON lines (no inter-leaving, no truncation)
//     and every chain link MUST verify in order.
//   - TestAudit_Rotation_OnSizeThreshold_Good — RotateAtSize=1KB forces
//     a sequenced sibling (.001.log) to appear after a handful of
//     events; chain re-seeds at the rotation boundary (genesis row in
//     the new file).
//   - TestAudit_ChainBreak_Detected_Bad — write 5 events, mutate the
//     middle row's __chain field on disk, expect VerifyChain to report
//     the break at line 3 with the expected vs actual digest pair.

package audit

import (
	"bytes"
	"sync"
	"testing"
	"time"

	core "dappco.re/go"
)

// TestAudit_ConcurrentWrite_NoCorruption_Good fires 16 goroutines at
// the recorder; the recorder's in-process Mutex + the chain ordering
// MUST produce a file where every line is valid JSON, every chain
// link verifies, and the total line count equals goroutines * each.
func TestAudit_ConcurrentWrite_NoCorruption_Good(t *testing.T) {
	const goroutines = 16
	const each = 25
	svc := newTestService(t)

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < each; i++ {
				r := svc.Record(Event{
					Event:   "queue.enqueued",
					TS:      core.Now().UTC().Unix(),
					Outcome: OutcomeOK,
				})
				if !r.OK {
					t.Errorf("Record failed: %s", r.Error())
					return
				}
			}
		}()
	}
	wg.Wait()

	now := core.Now().UTC().Unix()
	body := readDayFile(t, svc, now)
	got := bytes.Count(body, []byte{'\n'})
	want := goroutines * each
	if got != want {
		t.Fatalf("expected %d NDJSON lines, got %d", want, got)
	}
	// Every chain link MUST verify (no inter-leaving corruption).
	br, r := VerifyChain(svc.currentPath)
	if !r.OK {
		t.Fatalf("VerifyChain infrastructure failure: %s", r.Error())
	}
	if br != nil {
		t.Fatalf("chain break at line %d (expected %s, actual %s)",
			br.Line, br.Expected, br.Actual)
	}
}

// TestAudit_Rotation_OnSizeThreshold_Good asserts size-based rotation
// produces a sequenced sibling once RotateAtSize is crossed. The
// chain re-seeds in the new file (genesis-row digest), and the prior
// file remains intact with its own valid chain.
func TestAudit_Rotation_OnSizeThreshold_Good(t *testing.T) {
	root := t.TempDir()
	svc := New(nil, Options{
		Root:                  root,
		AuditSecret:           []byte("test-secret-fixture-deterministic"),
		ProcessLabel:          "lthn-test",
		RotationCheckInterval: time.Hour,
		RotateAtSize:          1024, // 1 KiB — easy to cross with a handful of events
		LockTimeout:           -1,
	})
	defer svc.Close()

	// Write ~30 events; with the default Meta + chain stamp each line
	// is ~150-200 bytes, so we cross 1 KiB at roughly the 5th-7th event.
	for i := 0; i < 30; i++ {
		r := svc.Record(Event{
			Event:   "queue.enqueued",
			TS:      core.Now().UTC().Unix(),
			Outcome: OutcomeOK,
			Meta:    map[string]any{"i": i, "payload": "padding-to-cross-rotation"},
		})
		if !r.OK {
			t.Fatalf("Record %d failed: %s", i, r.Error())
		}
	}

	// At least one sequenced sibling MUST exist on disk.
	entriesR := core.ReadDir(core.DirFS(root), ".")
	if !entriesR.OK {
		t.Fatalf("ReadDir failed: %s", entriesR.Error())
	}
	entries, _ := entriesR.Value.([]core.FsDirEntry)
	sawSeq := false
	for _, e := range entries {
		name := e.Name()
		if core.HasPrefix(name, dateStem(core.Now().UTC().Unix())+".001") {
			sawSeq = true
		}
	}
	if !sawSeq {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("expected a .001.log rotation sibling; saw: %v", names)
	}
	// Chain of the active (post-rotation) file MUST verify; the prior
	// file's chain is independent and also verifies. Walk both.
	for _, e := range entries {
		name := e.Name()
		if !core.HasSuffix(name, ".log") {
			continue
		}
		path := core.PathJoin(root, name)
		br, r := VerifyChain(path)
		if !r.OK {
			t.Fatalf("VerifyChain(%s) infrastructure failure: %s", name, r.Error())
		}
		if br != nil {
			t.Fatalf("chain break in %s at line %d (expected %s, actual %s)",
				name, br.Line, br.Expected, br.Actual)
		}
	}
}

// TestAudit_ChainBreak_Detected_Bad writes 5 events, mutates the
// middle row's __chain field on disk, and asserts VerifyChain reports
// the break at line 3 with the expected vs actual digest pair.
func TestAudit_ChainBreak_Detected_Bad(t *testing.T) {
	svc := newTestService(t)
	now := core.Now().UTC().Unix()
	for i := 0; i < 5; i++ {
		r := svc.Record(Event{
			Event:   EventAuthSessionIssued,
			TS:      now,
			Outcome: OutcomeOK,
			Meta:    map[string]any{"i": i},
		})
		if !r.OK {
			t.Fatalf("Record %d failed: %s", i, r.Error())
		}
	}
	path := svc.currentPath
	// Pre-mutation: chain MUST be intact.
	if br, r := VerifyChain(path); !r.OK || br != nil {
		t.Fatalf("baseline chain should verify; got br=%+v r.OK=%v", br, r.OK)
	}

	// Mutate the third line's __chain field to a known-wrong digest.
	bodyR := core.ReadFile(path)
	if !bodyR.OK {
		t.Fatalf("ReadFile failed: %s", bodyR.Error())
	}
	body, _ := bodyR.Value.([]byte)
	lines := bytes.Split(body, []byte{'\n'})
	// bytes.Split leaves a trailing empty after the terminating '\n'.
	// We want index 2 (third row, 0-indexed).
	if len(lines) < 5 {
		t.Fatalf("expected >=5 non-empty lines, got %d", len(lines))
	}
	var rec map[string]any
	if r := core.JSONUnmarshal(lines[2], &rec); !r.OK {
		t.Fatalf("unmarshal line 3 failed: %s", r.Error())
	}
	meta, _ := rec["meta"].(map[string]any)
	if meta == nil {
		t.Fatal("line 3 has no meta — fixture invariant broken")
	}
	// Replace the chain with a clearly-wrong digest of the same length
	// (64 hex chars) so the verifier's parse path doesn't reject it
	// for shape — only the chain-recomputation comparison should fire.
	meta[chainKey] = "deadbeef" +
		"deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbe"
	bR := core.JSONMarshal(rec)
	if !bR.OK {
		t.Fatalf("re-marshal line 3 failed: %s", bR.Error())
	}
	lines[2], _ = bR.Value.([]byte)
	mutated := bytes.Join(lines, []byte{'\n'})
	wR := core.WriteFile(path, mutated, 0o600)
	if !wR.OK {
		t.Fatalf("WriteFile (mutation) failed: %s", wR.Error())
	}

	br, vr := VerifyChain(path)
	if !vr.OK {
		t.Fatalf("VerifyChain infrastructure failure: %s", vr.Error())
	}
	if br == nil {
		t.Fatal("expected a ChainBreak at line 3, got nil (verifier missed the mutation)")
	}
	if br.Line != 3 {
		t.Fatalf("expected break at line 3, got line %d", br.Line)
	}
	if br.Actual != meta[chainKey].(string) {
		t.Fatalf("ChainBreak.Actual mismatch: want %s, got %s", meta[chainKey], br.Actual)
	}
}
