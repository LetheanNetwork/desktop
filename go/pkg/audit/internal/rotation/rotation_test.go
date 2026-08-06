// SPDX-Licence-Identifier: EUPL-1.2

package rotation

import (
	"testing"

	core "dappco.re/go"
)

// writeLog writes body to a fresh `<dir>/<stem>.log` and returns the
// full path. Fails the test on write error so callers can assume the
// file exists.
//
//	path := writeLog(t, t.TempDir(), "2026-05-15", "line one\nline two\n")
func writeLog(t *testing.T, dir, stem, body string) string {
	t.Helper()
	path := core.PathJoin(dir, stem+LogSuffix)
	if r := core.WriteFile(path, []byte(body), 0o600); !r.OK {
		t.Fatalf("WriteFile(%s): %v", path, r.Value)
	}
	return path
}

// readAll drains rd through OpenForRead and returns the decoded bytes.
func readAll(t *testing.T, path string) []byte {
	t.Helper()
	rd, closer, r := OpenForRead(path)
	if !r.OK {
		t.Fatalf("OpenForRead(%s): %s", path, r.Error())
	}
	defer closer()
	buf := make([]byte, 0, 256)
	chunk := make([]byte, 64)
	for {
		n, err := rd.Read(chunk)
		buf = append(buf, chunk[:n]...)
		if err != nil {
			break
		}
	}
	return buf
}

// --- Compress: Good ---

func TestRotation_Compress_Good_RoundTrips(t *testing.T) {
	dir := t.TempDir()
	body := "{\"event\":\"auth.ok\"}\n{\"event\":\"auth.fail\"}\n"
	inPath := writeLog(t, dir, "2026-05-15", body)

	gzPath, r := Compress(inPath, 0o600)
	if !r.OK {
		t.Fatalf("Compress: %s", r.Error())
	}
	want := core.PathJoin(dir, "2026-05-15"+CompressedSuffix)
	if gzPath != want {
		t.Fatalf("Compress returned %q, want %q", gzPath, want)
	}
	// Source removed after successful compress.
	if core.Stat(inPath).OK {
		t.Fatal("source .log still present after Compress — should be removed")
	}
	// Output decompresses back to the original bytes.
	got := readAll(t, gzPath)
	if string(got) != body {
		t.Fatalf("decompressed bytes mismatch; got %q want %q", got, body)
	}
}

// --- Compress: Bad ---

func TestRotation_Compress_Bad_WrongSuffix(t *testing.T) {
	dir := t.TempDir()
	// Path does NOT end in .log — must be rejected before any I/O.
	bad := core.PathJoin(dir, "2026-05-15.ndjson")
	if r := core.WriteFile(bad, []byte("x\n"), 0o600); !r.OK {
		t.Fatalf("WriteFile: %v", r.Value)
	}
	if _, r := Compress(bad, 0o600); r.OK {
		t.Fatal("Compress accepted a non-.log source — should reject on suffix")
	}
}

func TestRotation_Compress_Bad_MissingSource(t *testing.T) {
	dir := t.TempDir()
	missing := core.PathJoin(dir, "nonexistent"+LogSuffix)
	if _, r := Compress(missing, 0o600); r.OK {
		t.Fatal("Compress accepted a missing source — should fail to open")
	}
}

// TestRotation_Compress_Bad_DestPathIsDirectory — a directory already
// occupies the computed `.log.gz` output path (e.g. a stale partial
// rotation, or a directory squatting the name). OpenFile with
// O_CREATE|O_TRUNC on an existing directory fails (EISDIR) — Compress
// must surface that as the "open dest" failure, not panic or silently
// misbehave.
func TestRotation_Compress_Bad_DestPathIsDirectory(t *testing.T) {
	dir := t.TempDir()
	inPath := writeLog(t, dir, "2026-05-15", "body\n")
	gzPath := core.PathJoin(dir, "2026-05-15"+CompressedSuffix)
	if r := core.MkdirAll(gzPath, 0o700); !r.OK {
		t.Fatalf("pre-create dest as directory: %s", r.Error())
	}

	if _, r := Compress(inPath, 0o600); r.OK {
		t.Fatal("Compress accepted a destination path occupied by a directory")
	}
	// Source must be untouched — Compress failed before reaching the
	// remove-source step.
	if !core.Stat(inPath).OK {
		t.Fatal("Compress removed the source despite failing to open dest")
	}
}

// TestRotation_Compress_Bad_SourceIsDirectory — the computed source
// path is a directory rather than a regular file. OpenFile(O_RDONLY)
// on a directory succeeds (POSIX), but the subsequent io.Copy read
// fails ("is a directory") — Compress must surface that as the
// "stream copy" failure and clean up the partially-created dest.
func TestRotation_Compress_Bad_SourceIsDirectory(t *testing.T) {
	dir := t.TempDir()
	inPath := core.PathJoin(dir, "2026-05-15"+LogSuffix)
	if r := core.MkdirAll(inPath, 0o700); !r.OK {
		t.Fatalf("pre-create source as directory: %s", r.Error())
	}

	if _, r := Compress(inPath, 0o600); r.OK {
		t.Fatal("Compress accepted a directory as its source")
	}
	// Partially-created dest must be cleaned up on the stream-copy
	// failure path.
	gzPath := core.PathJoin(dir, "2026-05-15"+CompressedSuffix)
	if core.Stat(gzPath).OK {
		t.Fatal("Compress left a partial .log.gz behind after a stream-copy failure")
	}
}

// --- Compress: Ugly ---

func TestRotation_Compress_Ugly_EmptySource(t *testing.T) {
	// An empty `.log` is a legitimate edge: a day-file that rotated
	// before any event landed. Compress must still produce a valid,
	// empty-on-decode .log.gz and remove the source.
	dir := t.TempDir()
	inPath := writeLog(t, dir, "2026-01-01", "")

	gzPath, r := Compress(inPath, 0o600)
	if !r.OK {
		t.Fatalf("Compress empty source: %s", r.Error())
	}
	if core.Stat(inPath).OK {
		t.Fatal("empty source not removed after Compress")
	}
	got := readAll(t, gzPath)
	if len(got) != 0 {
		t.Fatalf("decompressed empty source non-empty; got %q", got)
	}
}

// --- Candidate: Good ---

func TestRotation_Candidate_Good_RenamesGz(t *testing.T) {
	dir := t.TempDir()
	gzPath := core.PathJoin(dir, "2026-02-13"+CompressedSuffix)
	if r := core.WriteFile(gzPath, []byte("gzbytes"), 0o600); !r.OK {
		t.Fatalf("WriteFile: %v", r.Value)
	}
	newPath, r := Candidate(gzPath)
	if !r.OK {
		t.Fatalf("Candidate: %s", r.Error())
	}
	want := gzPath + ".candidate"
	if newPath != want {
		t.Fatalf("Candidate returned %q, want %q", newPath, want)
	}
	if core.Stat(gzPath).OK {
		t.Fatal("original .log.gz still present after Candidate rename")
	}
	if !core.Stat(newPath).OK {
		t.Fatal(".log.gz.candidate missing after Candidate rename")
	}
}

// --- Candidate: Bad ---

func TestRotation_Candidate_Bad_WrongSuffix(t *testing.T) {
	dir := t.TempDir()
	bad := core.PathJoin(dir, "2026-02-13"+LogSuffix)
	if _, r := Candidate(bad); r.OK {
		t.Fatal("Candidate accepted a .log source — should reject (needs .log.gz)")
	}
}

// TestRotation_Candidate_Bad_RenameTargetOccupied — the computed
// `.candidate` path is already occupied by a non-empty directory, so
// the rename itself fails at the OS level. Candidate must surface
// that failure via the typed "rename to .candidate" wrap rather than
// silently losing the source.
func TestRotation_Candidate_Bad_RenameTargetOccupied(t *testing.T) {
	dir := t.TempDir()
	gzPath := core.PathJoin(dir, "2026-02-13"+CompressedSuffix)
	if r := core.WriteFile(gzPath, []byte("gzbytes"), 0o600); !r.OK {
		t.Fatalf("WriteFile: %v", r.Value)
	}
	occupied := gzPath + ".candidate"
	if r := core.MkdirAll(occupied, 0o700); !r.OK {
		t.Fatalf("pre-create candidate target as directory: %s", r.Error())
	}
	if r := core.WriteFile(core.PathJoin(occupied, "squatter"), []byte("x"), 0o600); !r.OK {
		t.Fatalf("occupy candidate target dir: %s", r.Error())
	}

	if _, r := Candidate(gzPath); r.OK {
		t.Fatal("Candidate accepted a rename onto an occupied non-empty directory")
	}
	// Source must remain — rename failed, nothing should be lost.
	if !core.Stat(gzPath).OK {
		t.Fatal("Candidate lost the source .log.gz despite a failed rename")
	}
}

// --- Candidate: Ugly ---

func TestRotation_Candidate_Ugly_AlreadyCandidate(t *testing.T) {
	// Idempotent: a path already carrying the candidate suffix returns
	// unchanged without touching disk.
	dir := t.TempDir()
	already := core.PathJoin(dir, "2026-02-13"+CandidateSuffix)
	got, r := Candidate(already)
	if !r.OK {
		t.Fatalf("Candidate on already-candidate: %s", r.Error())
	}
	if got != already {
		t.Fatalf("Candidate mutated an already-candidate path; got %q want %q", got, already)
	}
}

// --- ResolveDayFile: Good ---

func TestRotation_ResolveDayFile_Good_PrefersLog(t *testing.T) {
	// When .log, .log.gz and .log.gz.candidate all exist for a stem,
	// the active uncompressed .log wins.
	dir := t.TempDir()
	stem := "2026-05-15"
	for _, suffix := range []string{LogSuffix, CompressedSuffix, CandidateSuffix} {
		p := core.PathJoin(dir, stem+suffix)
		if r := core.WriteFile(p, []byte("x"), 0o600); !r.OK {
			t.Fatalf("WriteFile(%s): %v", p, r.Value)
		}
	}
	path, ok := ResolveDayFile(dir, stem)
	if !ok {
		t.Fatal("ResolveDayFile returned ok=false when files exist")
	}
	want := core.PathJoin(dir, stem+LogSuffix)
	if path != want {
		t.Fatalf("ResolveDayFile prefers wrong suffix; got %q want %q", path, want)
	}
}

func TestRotation_ResolveDayFile_Good_FallsThroughToCandidate(t *testing.T) {
	// Only the retention-flagged candidate survives — resolver must
	// still find it after fall-through.
	dir := t.TempDir()
	stem := "2026-05-15"
	candPath := core.PathJoin(dir, stem+CandidateSuffix)
	if r := core.WriteFile(candPath, []byte("x"), 0o600); !r.OK {
		t.Fatalf("WriteFile: %v", r.Value)
	}
	path, ok := ResolveDayFile(dir, stem)
	if !ok {
		t.Fatal("ResolveDayFile failed to fall through to .candidate")
	}
	if path != candPath {
		t.Fatalf("ResolveDayFile got %q, want %q", path, candPath)
	}
}

// --- ResolveDayFile: Bad ---

func TestRotation_ResolveDayFile_Bad_EmptyArgs(t *testing.T) {
	if _, ok := ResolveDayFile("", "2026-05-15"); ok {
		t.Fatal("ResolveDayFile accepted empty root")
	}
	if _, ok := ResolveDayFile(t.TempDir(), ""); ok {
		t.Fatal("ResolveDayFile accepted empty dateStem")
	}
}

// --- ResolveDayFile: Ugly ---

func TestRotation_ResolveDayFile_Ugly_NoFilesForDay(t *testing.T) {
	// No file for the stem is a normal "no events that day" outcome,
	// NOT an error — the resolver signals it via ok=false, empty path.
	dir := t.TempDir()
	path, ok := ResolveDayFile(dir, "1999-12-31")
	if ok {
		t.Fatalf("ResolveDayFile returned ok=true for a day with no files; path=%q", path)
	}
	if path != "" {
		t.Fatalf("ResolveDayFile returned non-empty path on miss; got %q", path)
	}
}

// --- OpenForRead: Good ---

func TestRotation_OpenForRead_Good_PlainLog(t *testing.T) {
	dir := t.TempDir()
	body := "plain ndjson line\n"
	path := writeLog(t, dir, "2026-05-15", body)
	got := readAll(t, path)
	if string(got) != body {
		t.Fatalf("OpenForRead plain .log mismatch; got %q want %q", got, body)
	}
}

func TestRotation_OpenForRead_Good_Compressed(t *testing.T) {
	dir := t.TempDir()
	body := "compressed ndjson line\n"
	inPath := writeLog(t, dir, "2026-05-15", body)
	gzPath, r := Compress(inPath, 0o600)
	if !r.OK {
		t.Fatalf("Compress: %s", r.Error())
	}
	got := readAll(t, gzPath)
	if string(got) != body {
		t.Fatalf("OpenForRead .log.gz mismatch; got %q want %q", got, body)
	}
}

// --- OpenForRead: Bad ---

func TestRotation_OpenForRead_Bad_MissingFile(t *testing.T) {
	dir := t.TempDir()
	missing := core.PathJoin(dir, "ghost"+LogSuffix)
	rd, closer, r := OpenForRead(missing)
	if r.OK {
		closer()
		t.Fatal("OpenForRead accepted a missing file — should fail to open")
	}
	if rd != nil {
		t.Fatal("OpenForRead returned a non-nil reader on failure")
	}
}

// --- OpenForRead: Ugly ---

func TestRotation_OpenForRead_Ugly_CandidateSuffixDecompresses(t *testing.T) {
	// A retention-flagged file keeps its gzip body — OpenForRead must
	// transparently decompress .log.gz.candidate the same as .log.gz.
	dir := t.TempDir()
	body := "retention-flagged but operator-readable\n"
	inPath := writeLog(t, dir, "2026-02-13", body)
	gzPath, r := Compress(inPath, 0o600)
	if !r.OK {
		t.Fatalf("Compress: %s", r.Error())
	}
	candPath, r := Candidate(gzPath)
	if !r.OK {
		t.Fatalf("Candidate: %s", r.Error())
	}
	got := readAll(t, candPath)
	if string(got) != body {
		t.Fatalf("OpenForRead .candidate mismatch; got %q want %q", got, body)
	}
}

// --- OpenForRead: Ugly corruption ---

func TestRotation_OpenForRead_Ugly_CorruptGzipHeader(t *testing.T) {
	// A `.log.gz` whose bytes are not a valid gzip stream must surface
	// a Fail from the gzip reader init, not panic.
	dir := t.TempDir()
	corrupt := core.PathJoin(dir, "2026-05-15"+CompressedSuffix)
	if r := core.WriteFile(corrupt, []byte("not gzip at all"), 0o600); !r.OK {
		t.Fatalf("WriteFile: %v", r.Value)
	}
	rd, closer, r := OpenForRead(corrupt)
	if r.OK {
		closer()
		t.Fatal("OpenForRead accepted a corrupt gzip header — should fail to init reader")
	}
	if rd != nil {
		t.Fatal("OpenForRead returned a non-nil reader on gzip-init failure")
	}
}
