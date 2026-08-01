<!-- SPDX-License-Identifier: EUPL-1.2 -->

# `io.Medium` repository audit

## Reproduction

- Date: 2026-07-26
- Baseline commit: `73c26e3c4f85251a568eb325efbde89cc46a08f2`
- Scope: production and test Go under `go/`, plus renderer TypeScript under
  `frontend/src`
- Pinned provider boundary: `dappco.re/go/io v0.15.3`

The Go inventory used an opening parenthesis after named methods so
`core.Stat(` does not falsely match HTTP constants such as `core.StatusOK`:

```bash
rg -n --glob '*.go' \
  '(^|[^[:alnum:]_])(os\.(Open|ReadFile|WriteFile|Create|Remove|RemoveAll|Rename|Stat|ReadDir)\(|filepath\.|syscall\.|core\.(ReadFile|WriteFile|ReadDir|DirFS|Stat|Lstat|MkdirAll)\()' \
  go

rg -n --glob '*.ts' \
  '(absolutePath|file://|Files\.Read|ListLocations|ListRecent|GetDiskUsage)' \
  frontend/src
```

The Go command found 55 package directories. `go/pkg/office/files` is not one
of them: its source guard also rejects raw filesystem/Core-Fs imports and
selectors. The TypeScript command found only two adversarial test fixtures
which prove that the strict Files bridge rejects `absolutePath`; there is no
production renderer match.

## Migration order

`P1` protects credentials and durable security records, `P2` protects
user-authored content, `P3` covers other product metadata, `P4` covers
artefacts and extension inputs, `P5` covers platform/build adapters, and `P6`
is test-only cleanup. A target owner is a logical registered Medium or audited
provider capability, not a renderer-selected host path.

### Security, identity, and boundary infrastructure

| Package | Current access | Target Medium or owner | Risk | Order |
|---|---|---|---|---|
| `go/pkg/account` | Core-Fs account and key paths | encrypted account Medium | boundary | P1 |
| `go/pkg/audit` | Core-Fs append/query files | append-safe audit Medium | metadata | P1 |
| `go/pkg/audit/internal/rotation` | path probes during rotation | audit Medium rotation capability | metadata | P1 |
| `go/pkg/bridge` | generic raw-path bridge helpers | remove renderer path authority; registered mount façade | boundary | P1 |
| `go/pkg/firstlaunch` | marker probes and reads | application metadata Medium | metadata | P1 |
| `dappco.re/go/crypt/keys` (wired by `go/pkg/keysvc`) | Core-Fs key tiers and migrations | encrypted key-vault Medium | boundary | P1 |
| `go/pkg/office/internal/safedir` | `Lstat` plus `MkdirAll` path guard | retire behind provider containment | boundary | P1 |
| `go/pkg/paths` | layout, atomic files, locks, `/proc`, and Darwin syscalls | application-data Medium plus read-only platform capability | boundary | P1 |
| `go/pkg/serverkey` | seed, token, and wallet paths | encrypted server-identity Medium | boundary | P1 |

### User-authored and office content

| Package | Current access | Target Medium or owner | Risk | Order |
|---|---|---|---|---|
| `go/pkg/chathistory` | Core-Fs exports and history directories | conversation/export Mediums | metadata | P2 |
| `go/pkg/incidents` | Core-Fs incident trees and Markdown | incidents content Medium | boundary | P2 |
| `go/pkg/marketing/audience` | Core-Fs records and Markdown | audience workspace Medium | boundary | P2 |
| `go/pkg/marketing/campaigns` | Core-Fs records and Markdown | campaign workspace Medium | boundary | P2 |
| `go/pkg/marketing/content` | Core-Fs records and Markdown | content workspace Medium | boundary | P2 |
| `go/pkg/marketing/social` | Core-Fs records and Markdown | social workspace Medium | boundary | P2 |
| `go/pkg/office/documents` | Core-Fs traversal/read/write | registered documents Medium | boundary | P2 |
| `go/pkg/office/mail` | Core-Fs mail bodies, threads, and attachments | mail-store and attachment Mediums | boundary | P2 |
| `go/pkg/runbooks` | Core-Fs runbook trees and Markdown | runbook workspace Medium | boundary | P2 |
| `go/pkg/sales/contacts` | Core-Fs contact records | contacts workspace Medium | boundary | P2 |
| `go/pkg/sales/deals` | Core-Fs deal records and Markdown | deals workspace Medium | boundary | P2 |
| `go/pkg/sales/forecast` | Core-Fs forecast records | forecast workspace Medium | metadata | P2 |
| `go/pkg/sales/pipeline` | Core-Fs pipeline records | pipeline workspace Medium | metadata | P2 |

### Product runtime metadata and catalogues

| Package | Current access | Target Medium or owner | Risk | Order |
|---|---|---|---|---|
| `go/pkg/deploys` | Core-Fs deployment definitions | deployments metadata Medium | metadata | P3 |
| `go/pkg/fleet` | state/database path copy and probes | fleet runtime Medium with snapshot capability | metadata | P3 |
| `go/pkg/lemma` | Core-Fs configuration and atomic writes | lemma configuration Medium | metadata | P3 |
| `go/pkg/marketplace` | manifests, cache, signing, and environment files | registry/cache/signing Mediums | boundary | P3 |
| `go/pkg/models` | model roots plus Unix rename/capacity syscalls | model-content Medium plus capacity capability | boundary | P3 |
| `go/pkg/queue` | database-path mode probe | queue runtime Medium | metadata | P3 |
| `go/pkg/r1` | model records and directories | R1 model-state Medium | metadata | P3 |
| `go/pkg/seeds` | seed discovery and reads | seed catalogue Medium | boundary | P3 |
| `go/pkg/sessions` | session documents and directories | sessions metadata Medium | metadata | P3 |
| `go/pkg/training` | checkpoint probes and reads | training/checkpoint Medium | boundary | P3 |

### Artefacts, extensions, and developer tooling

| Package | Current access | Target Medium or owner | Risk | Order |
|---|---|---|---|---|
| `go/cmd/lthn` | CLI imports, fleet copies, seeds, and migrations | delegate to registered product/tooling services | build tooling | P4 |
| `go/pkg/agents` | executable/repository probes | read-only developer-workspace Medium | build tooling | P4 |
| `go/pkg/build` | path existence probes | build-workspace Medium | build tooling | P4 |
| `go/pkg/calibrate` | executable probes | toolchain discovery capability | build tooling | P5 |
| `go/pkg/downloader` | quarantine directory and no-follow syscall | quarantine Medium with exclusive-create capability | boundary | P4 |
| `go/pkg/integrations` | configuration path probes | integration configuration Medium | metadata | P4 |
| `go/pkg/lint` | source path probes | read-only source-workspace Medium | build tooling | P4 |
| `go/pkg/opencode` | host imports, signatures, profiles, and studio paths | authorised developer-workspace/config Mediums | boundary | P4 |
| `go/pkg/php` | Composer and executable paths | read-only project/toolchain Mediums | build tooling | P4 |
| `go/pkg/plugin` | manifests, install trees, and local inputs | signed bundle and plugin-data Mediums | boundary | P4 |
| `go/pkg/repos` | repository-root traversal | registered repository Medium | boundary | P4 |
| `go/pkg/sandbox` | workspace parent creation | sandbox-workspace Medium | boundary | P4 |
| `go/pkg/sandwich` | kernel/signature reads | verified artefact Medium | boundary | P4 |
| `go/pkg/services` | launchd/systemd unit paths | audited host-service manager capability | build tooling | P5 |
| `go/pkg/tasks` | project manifest probes | read-only project Medium | build tooling | P4 |
| `go/pkg/terminal` | repository discovery | registered working-directory Medium metadata | build tooling | P4 |

### Test-only matches

| Package | Current access | Target Medium or owner | Risk | Order |
|---|---|---|---|---|
| `go/pkg/ai` | test fixture filesystem | Memory Medium or `t.TempDir()` provider test | test only | P6 |
| `go/pkg/appconfig` | test fixture filesystem | Memory Medium | test only | P6 |
| `go/pkg/contentshield` | test fixture filesystem | Memory Medium | test only | P6 |
| `go/pkg/lint/cousinvalidator` | test source tree | Memory or sandboxed test Medium | test only | P6 |
| `go/pkg/lint/serviceingress` | test source tree | Memory or sandboxed test Medium | test only | P6 |
| `go/pkg/ml` | test fixture filesystem | Memory Medium | test only | P6 |
| `go/pkg/openaibench` | test fixture filesystem | Memory Medium | test only | P6 |
| `go/pkg/runner` | test fixture filesystem | Memory Medium | test only | P6 |
| `go/pkg/vi` | test fixture filesystem | Memory Medium | test only | P6 |

## Current conclusion

The canonical Files data plane is guarded: it has one registered Wails
service, renderer requests carry only mount IDs and relative paths, persistent
runtime state uses a dedicated Medium, local roots use `go-io`'s `os.Root`
provider, and source tests reject a raw fallback.

The repository is **not yet globally sealed**. The packages above are a
pre-existing migration backlog; this audit intentionally does not convert
their storage contracts mechanically. New file-backed product work must use a
registered `io.Medium` immediately, and each older package needs an owned
contract, migration tests, and a source guard before its row can be closed.
