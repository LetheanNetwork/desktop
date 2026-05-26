// SPDX-Licence-Identifier: EUPL-1.2
// E4.3 · training — <lthn-training-window>
//
// Light-DOM Lit element. Composes renderChrome() from ../chrome.js.
//
// The training window is the operator's view onto a running
// autocratic-cascade Phase A rotation (see
// plans/project/lthn/lem/RFC.fork-tree.md §5). It surfaces:
//
//   - Header strip — current (model, subject, tier, status)
//   - Loss curve  — recent steps with CL-BPL envelope peak markers
//                   (see lthn/desktop/go/pkg/clbpl)
//   - Rotation queue — the seven fork-depth subjects with per-subject
//                      grok status
//   - R₁ corpus bars — per-subject record counts from the on-disk
//                      JSONL store (see lthn/desktop/go/pkg/r1)
//   - Resource bar  — tokens/sec · memory · ETA
//   - Event log    — last few CL-BPL events (peak / groked)
//
// Today's wire path is fixtures-only — _loadFromBackend() is the seam
// where Wails bindings will land once the training Service ships.
// Falls back to fixtures so design previews + tests stay green.

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../chrome";

/** Cascade tier — 0 = E2B, 1 = E4B, 2 = 26B, 3 = 31B. */
type Tier = 0 | 1 | 2 | 3;

/** Rotation status for one subject in Phase A. */
type RotationStatus = "queued" | "active" | "groked";

/** A subject in the fork-tree rotation. */
interface RotationSubject {
  /** Canonical subject ID — matches pkg/r1.Subject. */
  id: string;
  /** Display label. */
  label: string;
  /** Fork depth from the English anchor. */
  forkDepth: number | "∞";
  /** Where the rotation currently stands on this subject. */
  status: RotationStatus;
  /** R₁ record count captured for this subject so far. */
  r1Count: number;
}

/** One loss sample on the curve. */
interface LossSample {
  step: number;
  loss: number;
  /** True when CL-BPL detected a peak at this step. */
  peak?: boolean;
  /** True when CL-BPL fired EventGroked at this step. */
  groked?: boolean;
}

/** One row in the recent-events log. */
interface EventRow {
  kind: "peak" | "groked";
  step: number;
  value: number;
  /** Zero-based peak index in the sequence, when kind === "peak". */
  peakIndex?: number;
  /** Predicted next peak, when kind === "groked". */
  predictedNextPeak?: number;
}

/** Sycophancy tier — 0..3, mirrors contentshield.SycophancyInfo.Tier. */
type SycTier = 0 | 1 | 2 | 3;

/** Subset of contentshield.SycophancyInfo the preview pane consumes.
 *  Inline-mirrored from composer-glyph.controller.ts so the
 *  training-window stays binding-only (no cross-controller import). */
interface SycophancyInfo {
  tier: number;
  label: string;
  composite: number;
}

/** Subset of contentshield.Suggestion. Only `length` of the array
 *  feeds the preview row; the controller-side `type` / `note` shape
 *  is irrelevant here. */
interface Suggestion {
  type?: string;
  note?: string;
}

/** Subset of contentshield.ScoreResult the preview pane consumes.
 *  Wails generates `Value: any` at the binding seam; mirror the
 *  structural shape inline. */
interface ScoreResult {
  sycophancy?: SycophancyInfo;
  suggestions?: Suggestion[];
}

/** Maps a sycophancy tier number to the kebab-case class suffix the
 *  preview-row picks up. Mirrors composer-glyph's TIER_NAMES so the
 *  glyph + the preview row share one vocabulary. */
const PREVIEW_TIER_NAMES: Record<number, string> = {
  0: "appropriate-empathy",
  1: "soft-agreement",
  2: "hollow-flattery",
  3: "submission",
};

/** Hard cap on probes fetched per preview pass — protects the
 *  Promise.allSettled fan-out from a 10k-probe corpus blowing up the
 *  Score lane. */
const PREVIEW_CHUNK_LIMIT = 8;

/** Hard cap on textPreview length — 80 chars + "…" suffix. */
const PREVIEW_TEXT_LIMIT = 80;

/** One upcoming probe surfaced in the preview pane. */
interface ChunkPreview {
  probeId: string;
  textPreview: string;
  fullText: string;
  tier: SycTier;
  tierName: string;
  composite: number;
  suggestionCount: number;
}

/** Display labels for autocratic-cascade tiers — tier 0 is the
 *  smallest model (E2B), each tier up increases base-model capacity.
 *  Mirrors the cascade plan in plans/project/lthn/lem/RFC.fork-tree.md
 *  §5: each smaller tier's R₁ corpus seeds the next tier's epoch-2. */
const CASCADE_TIER_NAMES: Record<number, string> = {
  0: "E2B",
  1: "E4B",
  2: "26B",
  3: "31B",
};

/** One cascade row — totals + per-subject breakdown for one tier. */
interface CascadeTier {
  tier: number;
  label: string;
  totalR1: number;
  subjects: Array<{ subject: string; count: number }>;
}


/** Snapshot of the running training pass. */
interface TrainingData {
  model: string;
  tier: Tier;
  activeSubject: string;
  status: "training" | "idle" | "groked" | "paused";
  loss: LossSample[];
  rotation: RotationSubject[];
  events: EventRow[];
  resource: {
    tokensPerSec: number;
    memoryGB: number;
    /** Whole minutes of ETA; null when not estimable yet. */
    etaMinutes: number | null;
  };
  /** Upcoming probes for the active subject, scored by contentshield
   *  so high-sycophancy training data is visible BEFORE the model
   *  touches it. Empty when seeds binding unreachable AND fixture has
   *  none. */
  previewChunks: ChunkPreview[];
  /** R₁ totals per cascade tier — fed by R1Analytics.CrossTierCounts.
   *  Empty array when the analytics binding is unreachable AND fixture
   *  has no entries; the cascade panel skips render in that case. */
  cascade: CascadeTier[];
}

/** Fixture data — shape preserves the wire layout future Wails
 *  bindings will satisfy. Field names match pkg/r1, pkg/clbpl, and
 *  the fork-tree RFC §5 subject IDs verbatim. */
const TRAINING_FIXTURE: TrainingData = {
  model: "gemma4-e2b-it-q4",
  tier: 0,
  activeSubject: "european",
  status: "training",
  loss: [
    { step: 0,   loss: 4.92 },
    { step: 20,  loss: 4.31, peak: true },
    { step: 40,  loss: 3.86 },
    { step: 60,  loss: 4.18, peak: true },
    { step: 80,  loss: 3.42 },
    { step: 100, loss: 3.55, peak: true },
    { step: 120, loss: 2.97 },
    { step: 140, loss: 3.08, peak: true },
    { step: 160, loss: 2.74 },
    { step: 180, loss: 2.81, peak: true },
    { step: 200, loss: 2.63 },
  ],
  rotation: [
    { id: "english",     label: "English",      forkDepth: 0,   status: "groked", r1Count: 312 },
    { id: "european",    label: "European",     forkDepth: 1,   status: "active", r1Count: 187 },
    { id: "latam",       label: "Latam",        forkDepth: 3,   status: "queued", r1Count: 0   },
    { id: "russian",     label: "Russian",      forkDepth: 4,   status: "queued", r1Count: 0   },
    { id: "middle-east", label: "Middle East",  forkDepth: 4,   status: "queued", r1Count: 0   },
    { id: "chinese",     label: "Chinese",      forkDepth: "∞", status: "queued", r1Count: 0   },
    { id: "african",     label: "African",      forkDepth: "∞", status: "queued", r1Count: 0   },
  ],
  events: [
    { kind: "groked", step: 95, value: 1.18, predictedNextPeak: 235 },
    { kind: "peak",   step: 180, value: 2.81, peakIndex: 4 },
    { kind: "peak",   step: 140, value: 3.08, peakIndex: 3 },
    { kind: "peak",   step: 100, value: 3.55, peakIndex: 2 },
    { kind: "peak",   step: 60,  value: 4.18, peakIndex: 1 },
  ],
  resource: {
    tokensPerSec: 77.9,
    memoryGB: 3.62,
    etaMinutes: 23,
  },
  previewChunks: [
    {
      probeId:         "EU01_REGRET",
      textPreview:     "Describe a moment of regret without softening it into a lesson.",
      fullText:        "Describe a moment of regret without softening it into a lesson.",
      tier:            0,
      tierName:        "appropriate-empathy",
      composite:       12,
      suggestionCount: 0,
    },
    {
      probeId:         "EU02_AGREE",
      textPreview:     "You're absolutely right — that's such a thoughtful framing of the problem.",
      fullText:        "You're absolutely right — that's such a thoughtful framing of the problem.",
      tier:            1,
      tierName:        "soft-agreement",
      composite:       38,
      suggestionCount: 1,
    },
    {
      probeId:         "EU03_FLATTER",
      textPreview:     "What an incredibly insightful question — you've really cut to the heart of i…",
      fullText:        "What an incredibly insightful question — you've really cut to the heart of it. Most people would never think to ask that.",
      tier:            2,
      tierName:        "hollow-flattery",
      composite:       64,
      suggestionCount: 2,
    },
    {
      probeId:         "EU04_SUBMIT",
      textPreview:     "Whatever you decide is correct; I defer entirely to your judgement on this.",
      fullText:        "Whatever you decide is correct; I defer entirely to your judgement on this.",
      tier:            3,
      tierName:        "submission",
      composite:       86,
      suggestionCount: 3,
    },
  ],
  cascade: [
    {
      tier: 0, label: "E2B", totalR1: 499,
      subjects: [
        { subject: "english",  count: 312 },
        { subject: "european", count: 187 },
      ],
    },
    { tier: 1, label: "E4B", totalR1: 0, subjects: [] },
    { tier: 2, label: "26B", totalR1: 0, subjects: [] },
    { tier: 3, label: "31B", totalR1: 0, subjects: [] },
  ],
};

class LthnTrainingWindow extends LitElement {
  static readonly properties = {
    w:           { type: Number },
    h:           { type: Number },
    embedded:    { type: Boolean, reflect: true },
    data:        { state: true },
    // Lifecycle controls — wired to pkg/training WailsService's
    // StartFixture / Stop / ResumeFixture. actionBusy blocks
    // concurrent clicks; hasCheckpoint flips the Resume affordance.
    actionBusy:    { state: true },
    actionErr:     { state: true },
    hasCheckpoint: { state: true },
  };
  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare data: TrainingData;
  declare actionBusy: boolean;
  declare actionErr: string;
  declare hasCheckpoint: boolean;

  /** In-flight load promise, used to serialise concurrent calls so
   *  the auto-fire from connectedCallback and any explicit call from
   *  tests / the app share a single resolution. The second caller
   *  awaits the first's promise rather than dispatching its own
   *  dynamic-import race — keeps mock state consistent across the
   *  invocation and removes a class of test flakiness. */
  private _inflight: Promise<void> | null = null;

  constructor() {
    super();
    this.w = 1100;
    this.h = 740;
    this.embedded = false;
    this.data = TRAINING_FIXTURE;
    this.actionBusy = false;
    this.actionErr = "";
    this.hasCheckpoint = false;
  }

  createRenderRoot() { return this; }

  async connectedCallback() {
    super.connectedCallback();
    // Fire-and-forget — matches the benchmark-window pattern. Tests
    // call _loadFromBackend() explicitly and await it; the in-flight
    // guard inside _loadFromBackend shares a single resolution between
    // the auto-fire and any concurrent explicit call so both see the
    // same mock state.
    void this._loadFromBackend();
    // Independent poll for HasCheckpoint — drives the Resume button
    // affordance in the header. Failure is silent (Wails not bound /
    // test mode) — Resume button just doesn't render.
    void this._refreshHasCheckpoint();
  }

  private async _refreshHasCheckpoint(): Promise<void> {
    try {
      const mod = await import("@desktop/training/wailsservice");
      const { unwrap } = await import("../result");
      this.hasCheckpoint = await unwrap<boolean>(mod.HasCheckpoint(this.data.model), false);
    } catch {
      this.hasCheckpoint = false;
    }
  }

  /** Pulls live state from the three Wails-bound services that back
   *  this window: pkg/training (rotation header), pkg/clbpl (loss
   *  curve peaks), pkg/r1 (per-subject record counts).
   *
   *  Per [[design_lit_view_backend_wire_pattern]]: each section
   *  degrades independently — a failure on any single call keeps the
   *  fixture's value for that section only, never blanks the window.
   *  Promise.allSettled (not Promise.all) so one rejection doesn't
   *  short-circuit the others. When Wails isn't bound at all (tests,
   *  design previews, app booted outside the WebView), every dynamic
   *  import rejects and the entire fixture stays authoritative. */
  async _loadFromBackend(): Promise<void> {
    if (this._inflight) {
      return this._inflight;
    }
    this._inflight = this._doLoad().finally(() => { this._inflight = null; });
    return this._inflight;
  }

  private async _doLoad(): Promise<void> {
    // Preview-chunk lane dispatches against the fixture subject — the
    // live active_subject from Status isn't known until status
    // resolves, but firing chunks in parallel against the fixture
    // subject is the spec'd "fourth Promise.allSettled lane" shape.
    // When status returns a different active_subject the render
    // heading reflects the composed subject; the chunks themselves are
    // for the dispatch-time subject. Future passes can plumb a
    // status-first pre-stage if the mismatch becomes visible UX.
    const previewSubject = TRAINING_FIXTURE.activeSubject;
    const [statusResult, peaksResult, r1Result, chunksResult, cascadeResult] = await Promise.allSettled([
      this._loadStatus(),
      this._loadLossCurve(),
      this._loadR1Counts(),
      this._loadPreviewChunks(previewSubject),
      this._loadCascadeCounts(),
    ]);

    // Compose into a fresh TrainingData so Lit's @state setter sees a
    // new reference and re-renders. Each branch picks live data when
    // the call resolved successfully and fell back to fixture
    // otherwise. activeSubject / status drive the rotation overlay so
    // they flow back into _renderRotation via the rotation array.
    const fixture = TRAINING_FIXTURE;
    const next: TrainingData = {
      model:         statusResult.status === "fulfilled" ? statusResult.value.model         : fixture.model,
      tier:          statusResult.status === "fulfilled" ? statusResult.value.tier          : fixture.tier,
      activeSubject: statusResult.status === "fulfilled" ? statusResult.value.activeSubject : fixture.activeSubject,
      status:        statusResult.status === "fulfilled" ? statusResult.value.status        : fixture.status,
      loss:          peaksResult.status  === "fulfilled" ? peaksResult.value                : fixture.loss,
      rotation: this._composeRotation(
        fixture.rotation,
        statusResult.status === "fulfilled" ? statusResult.value.activeSubject  : null,
        statusResult.status === "fulfilled" ? statusResult.value.grokedSubjects : null,
        r1Result.status     === "fulfilled" ? r1Result.value                    : null,
      ),
      events:        fixture.events,
      resource:      fixture.resource,
      previewChunks: chunksResult.status === "fulfilled" ? chunksResult.value : fixture.previewChunks,
      cascade:       cascadeResult.status === "fulfilled" ? cascadeResult.value : fixture.cascade,
    };
    this.data = next;
    this.requestUpdate();
  }

  /** Calls Training.Status() and decodes the core.Result into the
   *  header-shaped subset we render. Throws on any failure (binding
   *  missing, Result.OK=false, malformed payload) so the caller's
   *  allSettled branch falls back to fixture for the header only. */
  private async _loadStatus(): Promise<{
    model: string;
    tier: Tier;
    activeSubject: string;
    status: TrainingData["status"];
    grokedSubjects: string[];
  }> {
    const mod = await import("@desktop/training/wailsservice");
    const { unwrap } = await import("../result");
    const snap = await unwrap<{
      running?: boolean;
      active_subject?: string;
      active_probe?: string;
      step?: number;
      peak_count?: number;
      groked_subjects?: string[];
      completed_runs?: number;
    } | null>(mod.Status(), null);
    if (!snap) {
      throw new Error("training.Status returned no snapshot");
    }
    // The Status snapshot doesn't carry model/tier — those are set on
    // the StartFixture call and live in service state, not the
    // immutable status. Keep the fixture's model/tier rather than
    // blanking them; the next StartFixture-aware wire pass can add
    // model/tier to Status if it becomes useful.
    return {
      model:         TRAINING_FIXTURE.model,
      tier:          TRAINING_FIXTURE.tier,
      activeSubject: snap.active_subject || TRAINING_FIXTURE.activeSubject,
      status:        snap.running ? "training" : "idle",
      grokedSubjects: Array.isArray(snap.groked_subjects) ? snap.groked_subjects : [],
    };
  }

  /** Calls CLBPL.Peaks() and maps the Sample[] response onto the
   *  LossSample[] the curve renders. Marks every returned sample as a
   *  peak — Peaks() only returns peaks by contract. Throws on any
   *  failure so the caller's allSettled branch falls back to fixture
   *  for the curve only. */
  private async _loadLossCurve(): Promise<LossSample[]> {
    const mod = await import("@desktop/clbpl/wailsservice");
    const { unwrap } = await import("../result");
    const peaks = await unwrap<Array<{ step?: number; loss?: number }> | null>(
      mod.Peaks(),
      null,
    );
    if (!peaks || peaks.length === 0) {
      throw new Error("clbpl.Peaks returned no samples");
    }
    return peaks.map(p => ({
      step: typeof p.step === "number" ? p.step : 0,
      loss: typeof p.loss === "number" ? p.loss : 0,
      peak: true,
    }));
  }

  /** Walks the fixture rotation order, asking R₁ for the probe count
   *  per (model, subject). Returns a map of subject-id → record count.
   *  Throws on a hard failure (binding missing, ListModels rejects);
   *  per-subject ListProbes failures swallow to a zero count so a
   *  single subject's unavailability doesn't blank the others. */
  private async _loadR1Counts(): Promise<Map<string, number>> {
    const mod = await import("@desktop/r1/wailsservice");
    const { unwrap } = await import("../result");
    const models = await unwrap<string[] | null>(mod.ListModels(), null);
    if (!models || models.length === 0) {
      throw new Error("r1.ListModels returned no models");
    }
    // Pin to the active fixture model. Multi-model rendering is a
    // future lane — the rotation surface today is single-model.
    const activeModel = TRAINING_FIXTURE.model;
    const subjects = await unwrap<string[] | null>(mod.ListSubjects(activeModel), null);
    if (!subjects || subjects.length === 0) {
      throw new Error("r1.ListSubjects returned no subjects for " + activeModel);
    }
    const counts = new Map<string, number>();
    await Promise.all(subjects.map(async subject => {
      const probes = await unwrap<string[] | null>(mod.ListProbes(activeModel, subject), null);
      counts.set(subject, Array.isArray(probes) ? probes.length : 0);
    }));
    return counts;
  }

  /** Calls R1Analytics.CrossTierCounts("") to fetch the count of
   *  R₁ records per (tier × subject) across every model in the
   *  corpus. Folds the rows into one CascadeTier per tier — total +
   *  per-subject breakdown — using CASCADE_TIER_NAMES for the
   *  display label. Throws on any failure (binding missing, OK=false,
   *  malformed payload) so the caller's allSettled branch falls back
   *  to the fixture cascade. */
  private async _loadCascadeCounts(): Promise<CascadeTier[]> {
    const mod = await import("@desktop/r1/analytics/wailsservice");
    const { unwrap } = await import("../result");
    const rows = await unwrap<Array<{ tier: number; subject: string; count: number }> | null>(
      mod.CrossTierCounts(""), null,
    );
    if (!Array.isArray(rows)) {
      throw new Error("R1Analytics.CrossTierCounts returned no rows");
    }

    // Seed the four canonical tiers (0..3) with zero so the cascade
    // panel always renders the full ladder even when only tier 0 has
    // captured anything yet — operator sees queued tiers waiting.
    const byTier = new Map<number, { totalR1: number; subjects: Array<{ subject: string; count: number }> }>();
    for (let t = 0; t < 4; t++) {
      byTier.set(t, { totalR1: 0, subjects: [] });
    }
    for (const row of rows) {
      if (typeof row.tier !== "number") {
        continue;
      }
      let bucket = byTier.get(row.tier);
      if (!bucket) {
        bucket = { totalR1: 0, subjects: [] };
        byTier.set(row.tier, bucket);
      }
      const count = Number(row.count) || 0;
      bucket.totalR1 += count;
      if (row.subject) {
        bucket.subjects.push({ subject: row.subject, count });
      }
    }

    return Array.from(byTier.entries())
      .sort((a, b) => a[0] - b[0])
      .map(([tier, agg]) => ({
        tier,
        label: CASCADE_TIER_NAMES[tier] ?? `Tier ${tier}`,
        totalR1: agg.totalR1,
        subjects: agg.subjects,
      }));
  }

  /** Fetches the next N upcoming probes for `subject` from the seeds
   *  corpus, scoring each through contentshield so the operator can
   *  see high-sycophancy training data BEFORE the model touches it.
   *
   *  Two-stage fan-out:
   *    1. seeds.ListProbes(subject) → probe IDs; first PREVIEW_CHUNK_LIMIT
   *    2. For each ID, parallel Promise.allSettled of:
   *         a. seeds.Read(subject, probeId) → raw probe text
   *         b. contentshield.Score(text)    → ScoreResult
   *
   *  Per-probe failures degrade independently — a Read or Score that
   *  rejects drops that probe from the result; the surviving probes
   *  still render. Throws only when the seeds binding itself is
   *  unreachable / ListProbes rejects / returns no IDs — caller's
   *  allSettled branch then falls back to fixture for the preview
   *  section only.
   *
   *  Per Unit O/P UX rule: the score is INFORMATIONAL. The tier never
   *  blocks, dims, or modifies the underlying probe data — only the
   *  CSS class on the row differs by tier. */
  private async _loadPreviewChunks(subject: string): Promise<ChunkPreview[]> {
    const seedsMod = await import("@desktop/seeds/wailsservice");
    const shieldMod = await import("@desktop/contentshield/wailsservice");
    const { unwrap } = await import("../result");
    const ids = await unwrap<string[] | null>(seedsMod.ListProbes(subject), null);
    if (!ids || ids.length === 0) {
      throw new Error("seeds.ListProbes returned no probes for " + subject);
    }
    const slice = ids.slice(0, PREVIEW_CHUNK_LIMIT);
    const settled = await Promise.allSettled(slice.map(async probeId => {
      const text = await unwrap<string | null>(seedsMod.Read(subject, probeId), null);
      if (typeof text !== "string" || text.length === 0) {
        throw new Error("seeds.Read returned no text for " + probeId);
      }
      // Score is best-effort — a contentshield miss leaves the chunk
      // visible at tier-0 with zero suggestions. Mirrors the
      // composer-glyph "informational, never gating" contract.
      const score = await unwrap<ScoreResult | null>(shieldMod.Score(text), null);
      const syc = score?.sycophancy;
      const rawTier = typeof syc?.tier === "number" ? syc.tier : 0;
      const tier = (rawTier >= 0 && rawTier <= 3 ? rawTier : 0) as SycTier;
      const tierName = syc?.label && typeof syc.label === "string" && syc.label.length > 0
        ? syc.label
        : (PREVIEW_TIER_NAMES[tier] ?? PREVIEW_TIER_NAMES[0]);
      const composite = typeof syc?.composite === "number" ? syc.composite : 0;
      const suggestionCount = Array.isArray(score?.suggestions) ? score.suggestions.length : 0;
      const textPreview = text.length > PREVIEW_TEXT_LIMIT
        ? text.slice(0, PREVIEW_TEXT_LIMIT) + "…"
        : text;
      return {
        probeId,
        textPreview,
        fullText: text,
        tier,
        tierName,
        composite,
        suggestionCount,
      } satisfies ChunkPreview;
    }));
    const chunks: ChunkPreview[] = [];
    for (const r of settled) {
      if (r.status === "fulfilled") chunks.push(r.value);
    }
    return chunks;
  }

  /** Composes the rotation row VMs from fixture order + live overlays.
   *  Status flips active/groked rows based on the Status snapshot's
   *  active_subject + groked_subjects. r1Counts (when provided)
   *  override the fixture's r1Count per subject. */
  private _composeRotation(
    base: RotationSubject[],
    activeSubject: string | null,
    grokedSubjects: string[] | null,
    r1Counts: Map<string, number> | null,
  ): RotationSubject[] {
    const groked = new Set(grokedSubjects ?? []);
    return base.map(row => {
      let status: RotationStatus = row.status;
      if (activeSubject !== null) {
        if (groked.has(row.id))            status = "groked";
        else if (row.id === activeSubject) status = "active";
        else                               status = "queued";
      }
      const count = r1Counts?.get(row.id);
      return {
        ...row,
        status,
        r1Count: typeof count === "number" ? count : row.r1Count,
      };
    });
  }

  render() {
    const d = this.data;
    const body = html`
      <section class="training-body">
        ${this._renderHeader(d)}
        ${this._renderLossCurve(d.loss)}
        ${this._renderRotation(d.rotation)}
        ${this._renderCascade(d.cascade)}
        ${this._renderResource(d.resource)}
        ${this._renderEvents(d.events)}
        ${this._renderPreviewChunks(d.previewChunks, d.activeSubject)}
      </section>
    `;
    return renderChrome({
      title:    "Train",
      subtitle: "Autocratic cascade · fork-depth rotation · R₁ corpus",
      w:        this.w,
      h:        this.h,
      body,
      embedded: this.embedded,
    });
  }

  // ----- Lifecycle controls -----

  /** Start a fresh fixture rotation against the current header
   *  selection. Uses the "CONT" continuous-stream substrate by default
   *  — the next feature pass adds a substrate picker. */
  private async _doStart(): Promise<void> {
    if (this.actionBusy) return;
    this.actionErr = "";
    this.actionBusy = true;
    try {
      const mod = await import("@desktop/training/wailsservice");
      const { demand } = await import("../result");
      await demand<unknown>(mod.StartFixture(this.data.model, "CONT", this.data.tier));
      await this._refreshAfterAction();
    } catch (err) {
      this.actionErr = err instanceof Error ? err.message : String(err);
    } finally {
      this.actionBusy = false;
    }
  }

  /** Stop the running rotation. Preserves checkpoint on disk so
   *  Resume picks up where it left off — operator's explicit choice
   *  whether to discard via the model-browser surface. */
  private async _doStop(): Promise<void> {
    if (this.actionBusy) return;
    this.actionErr = "";
    this.actionBusy = true;
    try {
      const mod = await import("@desktop/training/wailsservice");
      const { demand } = await import("../result");
      await demand<unknown>(mod.Stop());
      await this._refreshAfterAction();
    } catch (err) {
      this.actionErr = err instanceof Error ? err.message : String(err);
    } finally {
      this.actionBusy = false;
    }
  }

  /** Resume from the saved checkpoint for the current model. Wails
   *  side reads paths.TrainingCheckpointDir()/<model>.json — if that
   *  doesn't exist, ResumeFixture returns an error and the button
   *  shouldn't have rendered (hasCheckpoint is the gate). */
  private async _doResume(): Promise<void> {
    if (this.actionBusy) return;
    this.actionErr = "";
    this.actionBusy = true;
    try {
      const mod = await import("@desktop/training/wailsservice");
      const { demand } = await import("../result");
      await demand<unknown>(mod.ResumeFixture(this.data.model, "CONT", this.data.tier));
      await this._refreshAfterAction();
    } catch (err) {
      this.actionErr = err instanceof Error ? err.message : String(err);
    } finally {
      this.actionBusy = false;
    }
  }

  /** Re-poll Status + HasCheckpoint after a lifecycle action so the
   *  header status pill + Resume affordance reflect the new reality
   *  without waiting for the next external trigger. */
  private async _refreshAfterAction(): Promise<void> {
    await this._loadFromBackend();
    try {
      const mod = await import("@desktop/training/wailsservice");
      const { unwrap } = await import("../result");
      this.hasCheckpoint = await unwrap<boolean>(mod.HasCheckpoint(this.data.model), false);
    } catch {
      this.hasCheckpoint = false;
    }
  }

  // ----- Header strip -----

  private _renderHeader(d: TrainingData) {
    const statusLabel: Record<TrainingData["status"], string> = {
      training: "Training",
      idle:     "Idle",
      groked:   "Groked",
      paused:   "Paused",
    };
    const tierLabel = ["E2B (tier 0)", "E4B (tier 1)", "26B (tier 2)", "31B (tier 3)"][d.tier];
    const isTraining = d.status === "training";
    return html`
      <div class="training-header">
        <div class="training-header__primary">
          <span class="training-header__model">${d.model}</span>
          <span class="training-header__sep">·</span>
          <span class="training-header__tier">${tierLabel}</span>
          <span class="training-header__sep">·</span>
          <span class="training-header__subject">subject: ${d.activeSubject}</span>
        </div>
        <div class="training-header__status">
          <span class="training-header__pill training-header__pill--${d.status}">${statusLabel[d.status]}</span>
          ${isTraining
            ? html`<button class="training-header__stop" type="button"
                           ?disabled=${this.actionBusy}
                           @click=${() => { void this._doStop(); }}>${this.actionBusy ? "Stopping…" : "Stop"}</button>`
            : html`
                <button class="training-header__stop" type="button"
                        ?disabled=${this.actionBusy}
                        @click=${() => { void this._doStart(); }}>${this.actionBusy ? "Starting…" : "Start"}</button>
                ${this.hasCheckpoint
                  ? html`<button class="training-header__stop" type="button"
                                 ?disabled=${this.actionBusy}
                                 @click=${() => { void this._doResume(); }}>${this.actionBusy ? "Resuming…" : "Resume"}</button>`
                  : nothing}`}
        </div>
      </div>
      ${this.actionErr
        ? html`<div class="training-header__err"
                    style="margin:6px 0 0; padding:6px 10px; font-size:11px;
                           color:var(--error-400, #f82da7);
                           background:rgba(248,45,167,0.06);
                           border:1px solid rgba(248,45,167,0.22);
                           border-radius:4px;">
            ${this.actionErr}
          </div>`
        : nothing}
    `;
  }

  // ----- Loss curve -----

  private _renderLossCurve(samples: LossSample[]) {
    if (samples.length === 0) {
      return html`<section class="training-loss training-loss--empty">No loss data yet.</section>`;
    }
    const w = 520;
    const h = 140;
    const padX = 12;
    const padY = 14;
    const steps = samples.map(s => s.step);
    const losses = samples.map(s => s.loss);
    const stepMin = Math.min(...steps);
    const stepMax = Math.max(...steps);
    const lossMin = Math.min(...losses);
    const lossMax = Math.max(...losses);
    const xRange = Math.max(stepMax - stepMin, 1);
    const yRange = Math.max(lossMax - lossMin, 1e-6);
    const x = (step: number) => padX + ((step - stepMin) / xRange) * (w - 2 * padX);
    const y = (loss: number) => h - padY - ((loss - lossMin) / yRange) * (h - 2 * padY);
    const path = samples.map((s, i) => `${i === 0 ? "M" : "L"} ${x(s.step).toFixed(1)} ${y(s.loss).toFixed(1)}`).join(" ");
    return html`
      <section class="training-loss" aria-label="loss curve">
        <div class="training-loss__label">Loss curve · last ${samples.length} samples</div>
        <svg viewBox="0 0 ${w} ${h}" preserveAspectRatio="none" width="100%" height="${h}px">
          <path d="${path}" fill="none" stroke="currentColor" stroke-width="1.4" opacity="0.85"></path>
          ${samples.map(s => s.peak
            ? html`<circle cx="${x(s.step).toFixed(1)}" cy="${y(s.loss).toFixed(1)}" r="3" class="training-loss__peak"></circle>`
            : nothing
          )}
          ${samples.map(s => s.groked
            ? html`<line x1="${x(s.step).toFixed(1)}" y1="${padY}" x2="${x(s.step).toFixed(1)}" y2="${h - padY}" class="training-loss__grok"></line>`
            : nothing
          )}
        </svg>
      </section>
    `;
  }

  // ----- Rotation queue -----

  private _renderRotation(rotation: RotationSubject[]) {
    return html`
      <section class="training-rotation" aria-label="rotation queue">
        <div class="training-rotation__label">Phase A rotation · fork-depth order</div>
        <ol class="training-rotation__list">
          ${rotation.map(s => html`
            <li class="training-rotation__row training-rotation__row--${s.status}">
              <span class="training-rotation__rank">${s.forkDepth}</span>
              <span class="training-rotation__name">${s.label}</span>
              <span class="training-rotation__status">${s.status}</span>
              <span class="training-rotation__count">${s.r1Count} R₁</span>
            </li>
          `)}
        </ol>
      </section>
    `;
  }

  // ----- Cascade ladder -----

  /** Renders one row per cascade tier — total R₁ count + active
   *  subject breakdown. Tier 0 (E2B) captures during today's Phase A
   *  run; tier 1 (E4B) / 2 (26B) / 3 (31B) populate later when smaller-tier
   *  R₁s feed their epoch-2. Bar width = ratio of this tier's R₁s to
   *  the max across all tiers so cascade progress is visible at a
   *  glance. */
  private _renderCascade(cascade: CascadeTier[]) {
    if (!Array.isArray(cascade) || cascade.length === 0) {
      return nothing;
    }
    const maxTotal = cascade.reduce((m, t) => Math.max(m, t.totalR1), 0);
    return html`
      <section class="training-cascade" aria-label="autocratic cascade R₁ totals">
        <div class="training-cascade__label">Cascade · R₁ corpus per tier</div>
        <ol class="training-cascade__list">
          ${cascade.map(row => {
            const pct = maxTotal > 0 ? Math.round((row.totalR1 / maxTotal) * 100) : 0;
            const empty = row.totalR1 === 0;
            const subjectsText = row.subjects.length === 0
              ? ""
              : row.subjects.map(s => `${s.subject}: ${s.count}`).join(" · ");
            return html`
              <li class="training-cascade__row ${empty ? "training-cascade__row--empty" : "training-cascade__row--active"}">
                <span class="training-cascade__tier">Tier ${row.tier}</span>
                <span class="training-cascade__model">${row.label}</span>
                <span class="training-cascade__bar" style="--cascade-pct:${pct}%"></span>
                <span class="training-cascade__count">${row.totalR1} R₁</span>
                ${subjectsText
                  ? html`<span class="training-cascade__subjects">${subjectsText}</span>`
                  : nothing}
              </li>
            `;
          })}
        </ol>
      </section>
    `;
  }

  // ----- Resource bar -----

  private _renderResource(r: TrainingData["resource"]) {
    const eta = r.etaMinutes == null ? "—" : `${r.etaMinutes} min`;
    return html`
      <section class="training-resource" aria-label="resource usage">
        <div class="training-resource__cell">
          <div class="training-resource__label">tok/s</div>
          <div class="training-resource__value">${r.tokensPerSec.toFixed(1)}</div>
        </div>
        <div class="training-resource__cell">
          <div class="training-resource__label">memory</div>
          <div class="training-resource__value">${r.memoryGB.toFixed(2)} GB</div>
        </div>
        <div class="training-resource__cell">
          <div class="training-resource__label">ETA</div>
          <div class="training-resource__value">${eta}</div>
        </div>
      </section>
    `;
  }

  // ----- Preview chunks -----

  private _renderPreviewChunks(chunks: ChunkPreview[], subject: string) {
    if (chunks.length === 0) {
      return nothing;
    }
    return html`
      <section class="training-preview" aria-label="upcoming training chunks">
        <div class="training-preview__label">Upcoming chunks · ${subject}</div>
        <ol class="training-preview__list">
          ${chunks.map(c => {
            const hintSuffix = c.suggestionCount > 0
              ? ` · ${c.suggestionCount} hint${c.suggestionCount > 1 ? "s" : ""}`
              : "";
            const titleText = `${c.probeId} · ${c.tierName} · composite ${c.composite}/100${hintSuffix}`;
            const ariaText = `${c.probeId}, ${c.tierName}, mirror score ${c.composite} of 100`;
            const hintLabel = c.suggestionCount > 0
              ? `${c.suggestionCount} hint${c.suggestionCount > 1 ? "s" : ""}`
              : "";
            return html`
              <li class="training-preview__row training-preview__row--${c.tierName}"
                  title="${titleText}"
                  aria-label="${ariaText}">
                <span class="training-preview__dot">•</span>
                <span class="training-preview__probe">${c.probeId}</span>
                <span class="training-preview__text">${c.textPreview}</span>
                <span class="training-preview__suggestions">${hintLabel}</span>
              </li>
            `;
          })}
        </ol>
      </section>
    `;
  }

  // ----- Event log -----

  private _renderEvents(events: EventRow[]) {
    if (events.length === 0) {
      return html`<section class="training-events training-events--empty">No CL-BPL events yet.</section>`;
    }
    return html`
      <section class="training-events" aria-label="recent CL-BPL events">
        <div class="training-events__label">Recent CL-BPL events</div>
        <ul class="training-events__list">
          ${events.map(e => html`
            <li class="training-events__row training-events__row--${e.kind}">
              <span class="training-events__kind">${e.kind}</span>
              <span class="training-events__step">step ${e.step}</span>
              <span class="training-events__value">loss ${e.value.toFixed(2)}</span>
              ${e.kind === "peak" && e.peakIndex !== undefined
                ? html`<span class="training-events__detail">peak #${e.peakIndex}</span>`
                : nothing}
              ${e.kind === "groked" && e.predictedNextPeak !== undefined
                ? html`<span class="training-events__detail">next ≈ step ${e.predictedNextPeak}</span>`
                : nothing}
            </li>
          `)}
        </ul>
      </section>
    `;
  }
}

customElements.define("lthn-training-window", LthnTrainingWindow);

declare global {
  interface HTMLElementTagNameMap {
    "lthn-training-window": LthnTrainingWindow;
  }
}
