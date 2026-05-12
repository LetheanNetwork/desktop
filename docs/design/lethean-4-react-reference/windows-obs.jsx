/* windows-obs.jsx — E2 observability + E3 integration + E4 future windows
 * Benchmark · Logs · Live telemetry · Integrations · MCP tools · Network · Distillation · Fleet
 */

/* ─────────────────────────────────────────────────────────────────
 * E2.1 · Benchmark window
 * ───────────────────────────────────────────────────────────────── */
function BenchmarkWindow({ w = 1000, h = 660 }) {
  const runs = [
    { ts: "2026-05-11 14:32",  model: "gemma-4-e2b",  pp: 4820, tg: 47.2, w: 8.4,  mem: "2.4 GB", here: true },
    { ts: "2026-05-11 09:14",  model: "gemma-4-e2b",  pp: 4780, tg: 46.8, w: 8.5,  mem: "2.4 GB" },
    { ts: "2026-05-10 18:02",  model: "llama-3.2-3b", pp: 3140, tg: 32.6, w: 11.8, mem: "3.6 GB" },
    { ts: "2026-05-09 21:55",  model: "phi-3.5-mini", pp: 3960, tg: 38.4, w: 9.6,  mem: "2.9 GB" },
    { ts: "2026-05-08 11:18",  model: "gemma-4-e2b",  pp: 4640, tg: 45.1, w: 8.6,  mem: "2.4 GB" },
  ];
  // tok/s curve over context length
  const curve = [
    { ctx: 128,   tg: 51.8 },
    { ctx: 512,   tg: 50.4 },
    { ctx: 1024,  tg: 48.6 },
    { ctx: 2048,  tg: 47.2 },
    { ctx: 4096,  tg: 43.1 },
    { ctx: 6144,  tg: 38.4 },
    { ctx: 8192,  tg: 33.6 },
  ];
  const cw = 880, ch = 220, pad = { l: 48, r: 18, t: 16, b: 28 };
  const xs = (c) => pad.l + (Math.log2(c / 128) / Math.log2(8192 / 128)) * (cw - pad.l - pad.r);
  const ys = (t) => pad.t + (1 - (t - 20) / (60 - 20)) * (ch - pad.t - pad.b);
  return (
    <LthnWindow title="Benchmark" subtitle="run · compare · export" width={w} height={h}
      toolbar={<>
        <SettingsSelect value="gemma-4-e2b · q4_k_m" />
        <WinBtn tone="ghost" size="sm">PP only</WinBtn>
        <WinBtn tone="ghost" size="sm">TG only</WinBtn>
        <WinBtn tone="ghost" size="sm">Both</WinBtn>
        <div style={{ flex: 1 }} />
        <WinBtn tone="primary" size="sm" icon={<i className="fa-solid fa-play" style={{ fontSize: 9 }} />}>Run</WinBtn>
        <WinBtn tone="ghost" size="sm" icon={<i className="fa-regular fa-file-arrow-down" style={{ fontSize: 10 }} />}>Export</WinBtn>
      </>}
      footer={<>5 runs on file · ~/.lthn/bench/results.jsonl · last run 47.2 tok/s · 8.4 W</>}
    >
      <div style={{ flex: 1, display: "flex", flexDirection: "column", minHeight: 0 }}>
        {/* Top: history table */}
        <div style={{ padding: "14px 22px 8px", display: "flex", flexDirection: "column", gap: 6 }}>
          <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
            <WinLabel>Recent runs · click to overlay on chart</WinLabel>
            <div style={{ fontFamily: "var(--font-mono)", fontSize: 10, color: "var(--fg-3)" }}>
              2 selected · compare mode
            </div>
          </div>
          <div style={{
            background: "rgba(255,255,255,0.025)",
            border: "1px solid rgba(255,255,255,0.06)",
            borderRadius: 8,
            fontFamily: "var(--font-mono)", fontSize: 11,
          }}>
            <div style={{
              display: "grid",
              gridTemplateColumns: "20px 1.4fr 1.4fr 0.8fr 0.9fr 0.8fr 0.8fr 60px",
              padding: "8px 14px",
              borderBottom: "1px solid rgba(255,255,255,0.05)",
              color: "var(--fg-3)", fontSize: 10, letterSpacing: "0.04em", textTransform: "uppercase",
            }}>
              <span></span><span>Timestamp</span><span>Model</span><span>PP tok/s</span><span>TG tok/s</span><span>Peak W</span><span>Mem</span><span></span>
            </div>
            {runs.map((r, i) => {
              const sel = i < 2;
              return (
                <div key={i} style={{
                  display: "grid",
                  gridTemplateColumns: "20px 1.4fr 1.4fr 0.8fr 0.9fr 0.8fr 0.8fr 60px",
                  padding: "8px 14px",
                  background: r.here ? "rgba(64,193,197,0.07)" : "transparent",
                  borderBottom: i < runs.length - 1 ? "1px solid rgba(255,255,255,0.04)" : "none",
                  color: "var(--fg-1)",
                  alignItems: "center",
                }}>
                  <span style={{
                    width: 12, height: 12, borderRadius: 3,
                    background: sel ? (i === 0 ? "var(--brand-400)" : "var(--violet-400, #a78bfa)") : "transparent",
                    border: sel ? "none" : "1.5px solid rgba(255,255,255,0.18)",
                  }} />
                  <span style={{ color: "var(--fg-2)", fontSize: 10.5 }}>{r.ts}</span>
                  <span style={{ color: "var(--fg-0)" }}>{r.model}</span>
                  <span>{r.pp.toLocaleString()}</span>
                  <span style={{ color: r.here ? "var(--brand-300)" : "var(--fg-0)" }}>{r.tg.toFixed(1)}</span>
                  <span>{r.w} W</span>
                  <span style={{ color: "var(--fg-2)" }}>{r.mem}</span>
                  <span style={{ textAlign: "right" }}>
                    {r.here && <span style={{
                      fontSize: 9, padding: "1px 6px", borderRadius: 999,
                      background: "rgba(64,193,197,0.10)",
                      border: "1px solid rgba(64,193,197,0.22)",
                      color: "var(--brand-300)", letterSpacing: "0.06em",
                    }}>LATEST</span>}
                  </span>
                </div>
              );
            })}
          </div>
        </div>

        {/* Chart */}
        <div style={{ flex: 1, padding: "8px 22px 18px", display: "flex", flexDirection: "column", minHeight: 0 }}>
          <div style={{ display: "flex", alignItems: "baseline", gap: 14, marginBottom: 6 }}>
            <WinLabel>tok/s vs context length</WinLabel>
            <span style={{ fontSize: 10.5, color: "var(--fg-3)" }}>· log scale on x</span>
          </div>
          <div style={{
            flex: 1,
            background: "rgba(0,0,0,0.20)",
            border: "1px solid rgba(255,255,255,0.05)",
            borderRadius: 8,
            padding: 8,
          }}>
            <svg viewBox={`0 0 ${cw} ${ch}`} width="100%" height="100%" preserveAspectRatio="none">
              {[60, 50, 40, 30, 20].map((y) => (
                <g key={y}>
                  <line x1={pad.l} x2={cw - pad.r} y1={ys(y)} y2={ys(y)} stroke="rgba(255,255,255,0.05)" />
                  <text x={pad.l - 8} y={ys(y) + 3} fill="rgba(255,255,255,0.35)" fontSize="10" textAnchor="end" fontFamily="ui-monospace, monospace">{y}</text>
                </g>
              ))}
              {[128, 512, 1024, 2048, 4096, 8192].map((c) => (
                <g key={c}>
                  <line x1={xs(c)} x2={xs(c)} y1={pad.t} y2={ch - pad.b} stroke="rgba(255,255,255,0.04)" />
                  <text x={xs(c)} y={ch - pad.b + 14} fill="rgba(255,255,255,0.40)" fontSize="9.5" textAnchor="middle" fontFamily="ui-monospace, monospace">{c >= 1024 ? `${c/1024}k` : c}</text>
                </g>
              ))}
              {/* Current run — teal */}
              <path d={"M " + curve.map((p) => `${xs(p.ctx)} ${ys(p.tg)}`).join(" L ")}
                stroke="var(--brand-400)" strokeWidth="2" fill="none" />
              {curve.map((p, i) => (
                <circle key={i} cx={xs(p.ctx)} cy={ys(p.tg)} r="3" fill="var(--brand-400)" />
              ))}
              {/* Previous run — violet, faded */}
              <path d="M 48 88 L 198 92 L 348 100 L 498 108 L 648 130 L 798 158 L 870 178"
                stroke="#a78bfa" strokeOpacity="0.55" strokeWidth="1.8" fill="none" strokeDasharray="3 3" />
              {/* Legend */}
              <g transform={`translate(${cw - pad.r - 200}, ${pad.t + 6})`}>
                <rect width="200" height="42" fill="rgba(0,0,0,0.30)" stroke="rgba(255,255,255,0.06)" rx="4" />
                <circle cx="14" cy="14" r="4" fill="var(--brand-400)" />
                <text x="26" y="18" fill="rgba(255,255,255,0.85)" fontSize="10" fontFamily="ui-monospace, monospace">gemma-4-e2b · today</text>
                <circle cx="14" cy="30" r="4" fill="#a78bfa" fillOpacity="0.6" />
                <text x="26" y="34" fill="rgba(255,255,255,0.65)" fontSize="10" fontFamily="ui-monospace, monospace">llama-3.2-3b · -2 d</text>
              </g>
              <text x={8} y={pad.t + 4} fill="rgba(255,255,255,0.45)" fontSize="9.5" fontFamily="ui-monospace, monospace" transform={`rotate(-90 10 ${pad.t + 4})`}>tok/s</text>
            </svg>
          </div>
        </div>
      </div>
    </LthnWindow>
  );
}

/* ─────────────────────────────────────────────────────────────────
 * E2.2 · Logs / activity window — tabs allowed (modes of same data)
 * ───────────────────────────────────────────────────────────────── */
function LogsWindow({ tab = "live", w = 1000, h = 660 }) {
  return (
    <LthnWindow title="Activity" subtitle="logs · history · power" width={w} height={h}
      toolbar={<>
        {[
          { id: "live",    label: "Live log",          icon: "fa-wave-square" },
          { id: "history", label: "Generation history", icon: "fa-clock-rotate-left" },
          { id: "power",   label: "Power history",      icon: "fa-bolt" },
        ].map((t) => (
          <WinBtn key={t.id} tone={t.id === tab ? "primary" : "ghost"} size="sm"
            icon={<i className={`fa-solid ${t.icon}`} style={{ fontSize: 10 }} />}
            active={t.id === tab}>
            {t.label}
          </WinBtn>
        ))}
        <div style={{ flex: 1 }} />
        {tab === "live" && <>
          <WinBtn tone="ghost" size="sm" icon={<i className="fa-solid fa-magnifying-glass" style={{ fontSize: 10 }} />}>Filter</WinBtn>
          <WinBtn tone="ghost" size="sm" icon={<i className="fa-solid fa-pause" style={{ fontSize: 10 }} />}>Pause</WinBtn>
        </>}
      </>}
      footer={<>
        {tab === "live"    && "streaming · 1,284 lines · 4 components · debug verbose=off"}
        {tab === "history" && "27 generations · last 7 days · 1.42M tokens · 142.6 Wh"}
        {tab === "power"   && "showing last 24h · sample 1 s · powermetrics backend"}
      </>}
    >
      {tab === "live"    && <LogsLive />}
      {tab === "history" && <LogsHistory />}
      {tab === "power"   && <LogsPower />}
    </LthnWindow>
  );
}

function LogsLive() {
  const lines = [
    { t: "14:32:08.412", c: "runner",  s: "info",  m: "loaded gemma-4-e2b-q4_k_m.gguf (2.1 GB) into Metal heap" },
    { t: "14:32:08.418", c: "runner",  s: "info",  m: "kv-cache allocated · 8192 ctx · 384 MB" },
    { t: "14:32:08.421", c: "api",     s: "info",  m: "HTTP server listening on 127.0.0.1:8000" },
    { t: "14:32:14.802", c: "api",     s: "info",  m: "POST /v1/chat/completions · model=gemma-4-e2b · stream=true" },
    { t: "14:32:14.804", c: "runner",  s: "debug", m: "tokenize · 142 tokens · cache hit @ prefix(64)" },
    { t: "14:32:14.811", c: "runner",  s: "info",  m: "prefill · 78 new tok · 4820 tok/s · 8.2 W" },
    { t: "14:32:14.831", c: "runner",  s: "info",  m: "decode · streaming · target 47.2 tok/s" },
    { t: "14:32:18.106", c: "telem",   s: "debug", m: "powermetrics sample · cpu 4.2 W · gpu 4.1 W · ane 0.1 W" },
    { t: "14:32:21.488", c: "runner",  s: "info",  m: "decode complete · 158 tok in 3.35s · 47.2 tok/s · finish=stop" },
    { t: "14:32:21.491", c: "api",     s: "info",  m: "response sent · 4.687s e2e · 8.4 W peak" },
    { t: "14:32:24.002", c: "tray",    s: "debug", m: "sparkline frame · 60 samples · idle 0.4 W" },
    { t: "14:33:02.114", c: "api",     s: "warn",  m: "rate-limit · 127.0.0.1 · 12 req/s · soft cap 8 — backing off" },
    { t: "14:33:08.802", c: "telem",   s: "debug", m: "powermetrics sample · cpu 0.3 W · gpu 0.0 W · ane 0.0 W" },
    { t: "14:34:11.502", c: "kernel",  s: "info",  m: "metal command-buffer · 142 ops · 18.4 ms" },
  ];
  const sevColor = { info: "var(--fg-2)", debug: "var(--fg-3)", warn: "var(--warning-400)", error: "var(--err-400)" };
  return (
    <div style={{ flex: 1, display: "grid", gridTemplateColumns: "180px 1fr", minHeight: 0 }}>
      {/* Filter rail */}
      <aside style={{
        background: "rgba(0,0,0,0.18)",
        borderRight: "1px solid rgba(255,255,255,0.05)",
        padding: "14px 12px",
        display: "flex", flexDirection: "column", gap: 12,
      }}>
        <div>
          <WinLabel>Components</WinLabel>
          <div style={{ marginTop: 8, display: "flex", flexDirection: "column", gap: 4 }}>
            {[
              { k: "runner", n: 428, on: true },
              { k: "api",    n: 612, on: true },
              { k: "telem",  n: 144, on: true },
              { k: "tray",   n: 62,  on: true },
              { k: "kernel", n: 38,  on: true },
            ].map((c) => (
              <div key={c.k} style={{ display: "flex", alignItems: "center", gap: 8, fontSize: 11 }}>
                <input type="checkbox" defaultChecked={c.on} style={{ accentColor: "var(--brand-400)" }} />
                <span style={{ fontFamily: "var(--font-mono)", color: "var(--fg-1)", flex: 1 }}>{c.k}</span>
                <span style={{ fontFamily: "var(--font-mono)", fontSize: 9.5, color: "var(--fg-3)" }}>{c.n}</span>
              </div>
            ))}
          </div>
        </div>
        <div>
          <WinLabel>Severity</WinLabel>
          <div style={{ marginTop: 8, display: "flex", flexDirection: "column", gap: 4 }}>
            {[
              { k: "error", c: "var(--err-400)",     on: true,  n: 0 },
              { k: "warn",  c: "var(--warning-400)", on: true,  n: 1 },
              { k: "info",  c: "var(--brand-300)",   on: true,  n: 8 },
              { k: "debug", c: "var(--fg-3)",        on: false, n: 5 },
            ].map((s) => (
              <div key={s.k} style={{ display: "flex", alignItems: "center", gap: 8, fontSize: 11 }}>
                <input type="checkbox" defaultChecked={s.on} style={{ accentColor: "var(--brand-400)" }} />
                <span style={{ width: 6, height: 6, borderRadius: "50%", background: s.c }} />
                <span style={{ color: "var(--fg-1)", flex: 1 }}>{s.k}</span>
                <span style={{ fontFamily: "var(--font-mono)", fontSize: 9.5, color: "var(--fg-3)" }}>{s.n}</span>
              </div>
            ))}
          </div>
        </div>
      </aside>
      {/* Log stream */}
      <main style={{
        overflow: "auto",
        padding: "8px 0",
        fontFamily: "var(--font-mono)", fontSize: 11.5, lineHeight: 1.6,
      }}>
        {lines.map((l, i) => (
          <div key={i} style={{
            display: "grid",
            gridTemplateColumns: "112px 64px 50px 1fr",
            padding: "1.5px 16px",
            background: l.s === "warn" ? "rgba(245,158,11,0.06)" : i % 2 === 0 ? "transparent" : "rgba(255,255,255,0.015)",
            gap: 10,
          }}>
            <span style={{ color: "var(--fg-3)" }}>{l.t}</span>
            <span style={{ color: "var(--fg-2)" }}>{l.c}</span>
            <span style={{ color: sevColor[l.s], letterSpacing: "0.04em", fontSize: 10, textTransform: "uppercase" }}>{l.s}</span>
            <span style={{ color: "var(--fg-1)", whiteSpace: "pre-wrap" }}>{l.m}</span>
          </div>
        ))}
        <div style={{
          padding: "8px 16px", display: "flex", alignItems: "center", gap: 8,
          fontSize: 10.5, color: "var(--fg-3)",
        }}>
          <span style={{ width: 6, height: 6, borderRadius: "50%", background: "var(--success-400)", boxShadow: "0 0 4px var(--success-400)" }} />
          live · waiting for next event…
        </div>
      </main>
    </div>
  );
}

function LogsHistory() {
  const gens = [
    { ts: "14:32:14",  model: "gemma-4-e2b",  tok: 158, tg: 47.2, w: 8.4,  prompt: "Rewrite this function to use streams instead of arrays…" },
    { ts: "12:08:42",  model: "gemma-4-e2b",  tok: 384, tg: 46.8, w: 8.3,  prompt: "Summarise the changes between v0.1 and v0.2-rc1 of the runner…" },
    { ts: "11:55:18",  model: "llama-3.2-3b", tok: 220, tg: 32.6, w: 11.8, prompt: "What's the difference between LoRA rank 8 and rank 16?" },
    { ts: "09:42:01",  model: "gemma-4-e2b",  tok: 642, tg: 45.9, w: 8.5,  prompt: "Draft a release note for the new model browser…" },
    { ts: "08:18:33",  model: "phi-3.5-mini", tok: 184, tg: 38.4, w: 9.6,  prompt: "Translate the following help-centre article to British English…" },
  ];
  return (
    <div style={{ flex: 1, padding: "12px 22px 18px", overflow: "auto" }}>
      <div style={{
        background: "rgba(255,255,255,0.025)",
        border: "1px solid rgba(255,255,255,0.06)",
        borderRadius: 8,
        fontFamily: "var(--font-mono)", fontSize: 11.5,
      }}>
        <div style={{
          display: "grid", gridTemplateColumns: "100px 1.3fr 0.6fr 0.6fr 0.6fr 2fr",
          padding: "10px 14px",
          borderBottom: "1px solid rgba(255,255,255,0.06)",
          color: "var(--fg-3)", fontSize: 10, letterSpacing: "0.04em", textTransform: "uppercase",
        }}>
          <span>Time</span><span>Model</span><span>Tokens</span><span>tok/s</span><span>Peak W</span><span>Prompt</span>
        </div>
        {gens.map((g, i) => (
          <div key={i} style={{
            display: "grid", gridTemplateColumns: "100px 1.3fr 0.6fr 0.6fr 0.6fr 2fr",
            padding: "10px 14px",
            borderBottom: i < gens.length - 1 ? "1px solid rgba(255,255,255,0.04)" : "none",
            alignItems: "center", gap: 8,
          }}>
            <span style={{ color: "var(--fg-2)" }}>{g.ts}</span>
            <span style={{ color: "var(--fg-0)" }}>{g.model}</span>
            <span style={{ color: "var(--fg-1)" }}>{g.tok}</span>
            <span style={{ color: "var(--brand-300)" }}>{g.tg}</span>
            <span style={{ color: "var(--fg-1)" }}>{g.w}</span>
            <span style={{ color: "var(--fg-2)", whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>{g.prompt}</span>
          </div>
        ))}
      </div>
      <div style={{ marginTop: 14, fontSize: 11, color: "var(--fg-3)", lineHeight: 1.55 }}>
        Local. Never leaves this Mac. Right-click a row to re-open it in chat or export the transcript.
      </div>
    </div>
  );
}

function LogsPower() {
  // 60 samples over 24h
  const samples = Array.from({ length: 60 }, (_, i) => {
    const base = 0.4 + Math.sin(i * 0.3) * 0.3;
    const spike = [12, 13, 14, 22, 23, 38, 39, 40, 50, 51].includes(i) ? 6 + Math.random() * 3 : 0;
    return Math.max(0.2, base + spike);
  });
  const w = 940, h = 280, pad = 32;
  const max = 12;
  return (
    <div style={{ flex: 1, padding: "12px 22px 18px", display: "flex", flexDirection: "column", gap: 14, overflow: "auto" }}>
      <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: 10 }}>
        {[
          { l: "24h average",  v: "1.8 W",  s: "≈ a USB-C trickle" },
          { l: "24h total",    v: "44.2 Wh", s: "≈ 8 phone-charges" },
          { l: "Peak today",   v: "9.4 W",  s: "during decode @ 14:32" },
        ].map((k) => (
          <div key={k.l} style={{
            padding: "14px 16px", borderRadius: 8,
            background: "rgba(255,255,255,0.03)",
            border: "1px solid rgba(255,255,255,0.06)",
          }}>
            <WinLabel>{k.l}</WinLabel>
            <div style={{ fontFamily: "var(--font-mono)", fontSize: 22, color: "var(--fg-0)", marginTop: 6, letterSpacing: "-0.01em" }}>{k.v}</div>
            <div style={{ fontSize: 11, color: "var(--fg-3)", marginTop: 4 }}>{k.s}</div>
          </div>
        ))}
      </div>
      <div style={{
        background: "rgba(0,0,0,0.20)",
        border: "1px solid rgba(255,255,255,0.05)",
        borderRadius: 8,
        padding: 12,
      }}>
        <WinLabel>Watts · last 24 hours</WinLabel>
        <svg viewBox={`0 0 ${w} ${h}`} width="100%" height={h} preserveAspectRatio="none" style={{ marginTop: 6 }}>
          {[0, 3, 6, 9, 12].map((v) => (
            <g key={v}>
              <line x1={pad} x2={w} y1={h - pad - (v / max) * (h - pad - 16)} y2={h - pad - (v / max) * (h - pad - 16)} stroke="rgba(255,255,255,0.04)" />
              <text x={pad - 6} y={h - pad - (v / max) * (h - pad - 16) + 3} fill="rgba(255,255,255,0.40)" fontSize="10" textAnchor="end" fontFamily="ui-monospace, monospace">{v} W</text>
            </g>
          ))}
          <path d={"M " + samples.map((s, i) => `${pad + (i / (samples.length - 1)) * (w - pad)} ${h - pad - (s / max) * (h - pad - 16)}`).join(" L ") + ` L ${w} ${h - pad} L ${pad} ${h - pad} Z`}
            fill="rgba(64,193,197,0.10)" />
          <path d={"M " + samples.map((s, i) => `${pad + (i / (samples.length - 1)) * (w - pad)} ${h - pad - (s / max) * (h - pad - 16)}`).join(" L ")}
            stroke="var(--brand-400)" strokeWidth="1.4" fill="none" />
          {["00:00", "06:00", "12:00", "18:00", "now"].map((t, i) => (
            <text key={t} x={pad + (i / 4) * (w - pad)} y={h - 8} fill="rgba(255,255,255,0.40)" fontSize="10" textAnchor={i === 4 ? "end" : "middle"} fontFamily="ui-monospace, monospace">{t}</text>
          ))}
        </svg>
        <div style={{ marginTop: 8, fontSize: 11, color: "var(--fg-3)", fontStyle: "italic", lineHeight: 1.5 }}>
          For comparison — a typical fridge averages ~150 W. A Christmas-tree bulb, ~5 W.
        </div>
      </div>
    </div>
  );
}

/* ─────────────────────────────────────────────────────────────────
 * E2.3 · Live telemetry window — the demo surface
 * ───────────────────────────────────────────────────────────────── */
function TelemetryWindow({ fullscreen = false, w = 880, h = 560 }) {
  const tokSpark = [38, 41, 44, 45, 46, 47.2, 47, 46.8, 47.1, 47.4, 47.2, 47.0, 47.3, 47.2, 47.4, 47.2, 47.1, 47.3, 47.2, 47.0];
  const wattSpark = [0.6, 0.8, 4.2, 7.8, 8.2, 8.4, 8.3, 8.4, 8.5, 8.4, 8.3, 8.4, 8.4, 8.5, 8.4, 8.3, 8.4, 8.4, 8.3, 8.2];
  return (
    <LthnWindow title="Live telemetry" subtitle={fullscreen ? "fullscreen · ⎋ to exit" : "demo surface"} width={w} height={h}
      footer={<>model · gemma-4-e2b · context 142 / 8192 · airplane-mode OK · ⌥⌘F for fullscreen</>}>
      <div style={{
        flex: 1,
        background: "radial-gradient(circle at 50% 35%, rgba(64,193,197,0.10) 0%, rgba(11,16,22,0) 60%), var(--surf-0)",
        display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center",
        padding: "40px 60px", gap: 36, position: "relative", overflow: "hidden",
      }}>
        {/* Big readouts */}
        <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 64, width: "100%" }}>
          <BigReadout label="tok/s"  value="47.2" sub="generation speed" glow="var(--brand-400)" sparkline={tokSpark} max={60} />
          <BigReadout label="watts" value="8.4"  sub="peak this turn"   glow="#a78bfa" sparkline={wattSpark} max={12} />
        </div>
        {/* Lower band */}
        <div style={{
          display: "flex", gap: 28, alignItems: "center",
          fontFamily: "var(--font-mono)", fontSize: 12, color: "var(--fg-2)",
          paddingTop: 8, borderTop: "1px solid rgba(255,255,255,0.05)",
          width: "100%", justifyContent: "center",
        }}>
          <div><span style={{ color: "var(--fg-3)" }}>model </span><span style={{ color: "var(--fg-0)" }}>gemma-4-e2b</span></div>
          <div style={{ width: 1, height: 14, background: "rgba(255,255,255,0.06)" }} />
          <div><span style={{ color: "var(--fg-3)" }}>context </span><span style={{ color: "var(--fg-0)" }}>142 / 8,192</span></div>
          <div style={{ width: 1, height: 14, background: "rgba(255,255,255,0.06)" }} />
          <div><span style={{ color: "var(--fg-3)" }}>quant </span><span style={{ color: "var(--fg-0)" }}>q4_k_m</span></div>
          <div style={{ width: 1, height: 14, background: "rgba(255,255,255,0.06)" }} />
          <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
            <span style={{ width: 6, height: 6, borderRadius: "50%", background: "var(--success-400)", boxShadow: "0 0 6px var(--success-400)" }} />
            <span style={{ color: "var(--success-400)" }}>airplane-mode OK</span>
          </div>
        </div>
        {/* Corner mark */}
        <div style={{
          position: "absolute", bottom: 18, right: 24,
          display: "flex", alignItems: "center", gap: 8,
          fontFamily: "var(--font-mono)", fontSize: 10, color: "var(--fg-3)", letterSpacing: "0.06em",
        }}>
          <LthnGlyph size={12} color="var(--fg-3)" />
          lthn · sovereign · single-watt
        </div>
      </div>
    </LthnWindow>
  );
}

function BigReadout({ label, value, sub, glow, sparkline, max }) {
  const w = 320, h = 36;
  return (
    <div style={{ display: "flex", flexDirection: "column", alignItems: "center", gap: 10 }}>
      <div style={{
        fontFamily: "var(--font-mono)", fontSize: 10.5,
        color: "var(--fg-3)", letterSpacing: "0.20em", textTransform: "uppercase",
      }}>{label}</div>
      <div style={{
        fontFamily: "var(--font-mono)", fontSize: 92, fontWeight: 300,
        color: "var(--fg-0)",
        letterSpacing: "-0.04em", lineHeight: 1,
        textShadow: `0 0 30px ${glow}55, 0 0 60px ${glow}22`,
      }}>{value}</div>
      <svg viewBox={`0 0 ${w} ${h}`} width={w} height={h}>
        <path d={"M " + sparkline.map((s, i) => `${(i / (sparkline.length - 1)) * w} ${h - (s / max) * h}`).join(" L ")}
          stroke={glow} strokeWidth="1.5" fill="none" />
        <path d={"M " + sparkline.map((s, i) => `${(i / (sparkline.length - 1)) * w} ${h - (s / max) * h}`).join(" L ") + ` L ${w} ${h} L 0 ${h} Z`}
          fill={glow} fillOpacity="0.10" />
      </svg>
      <div style={{ fontSize: 11.5, color: "var(--fg-3)" }}>{sub}</div>
    </div>
  );
}

/* ─────────────────────────────────────────────────────────────────
 * E3.1 · Integrations / clients window
 * ───────────────────────────────────────────────────────────────── */
function IntegrationsWindow({ w = 880, h = 660 }) {
  const clients = [
    { id: "claude-code", name: "Claude Code",  state: "connected",   desc: "Anthropic CLI · OpenAI-compatible endpoint mode",
      path: "~/.config/claude/config.json", lastPing: "8 s ago · 142 ms" },
    { id: "opencode",    name: "OpenCode",     state: "connected",   desc: "Open-source coding agent",
      path: "~/.config/opencode/config.toml", lastPing: "1 m ago · 158 ms" },
    { id: "codex",       name: "Codex CLI",    state: "disconnected", desc: "OpenAI CLI",
      path: "~/.codex/config.yaml" },
    { id: "copilot",     name: "GitHub Copilot",state: "disconnected", desc: "VS Code extension proxy mode",
      path: "~/Library/Application Support/Code/copilot/config.json" },
    { id: "pi",          name: "Pi (raycast)", state: "available",   desc: "Raycast extension talks to lthn directly",
      path: "(no config needed)" },
  ];
  const selected = clients[0];
  return (
    <LthnWindow title="Integrations" subtitle="clients · MCP · webhooks" width={w} height={h}
      footer={<>2 connected · 1 endpoint · http://localhost:8000/v1 · only outbound action lthn ever takes</>}>
      <div style={{ flex: 1, display: "grid", gridTemplateColumns: "260px 1fr", minHeight: 0 }}>
        {/* Client rail */}
        <aside style={{
          background: "rgba(0,0,0,0.18)",
          borderRight: "1px solid rgba(255,255,255,0.05)",
          padding: "12px 8px",
          display: "flex", flexDirection: "column", gap: 1,
        }}>
          <WinLabel style={{ padding: "4px 10px 8px" }}>Clients</WinLabel>
          {clients.map((c, i) => {
            const active = i === 0;
            const stateColor = c.state === "connected" ? "var(--success-400)" : c.state === "disconnected" ? "var(--fg-3)" : "var(--warning-400)";
            return (
              <div key={c.id} style={{
                padding: "10px 12px",
                borderRadius: 6,
                background: active ? "rgba(255,255,255,0.06)" : "transparent",
                borderLeft: active ? "2px solid var(--brand-400)" : "2px solid transparent",
                display: "flex", flexDirection: "column", gap: 4,
                cursor: "pointer",
              }}>
                <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                  <span style={{
                    width: 7, height: 7, borderRadius: "50%", background: stateColor,
                    boxShadow: c.state === "connected" ? `0 0 4px ${stateColor}` : "none",
                  }} />
                  <span style={{ fontSize: 12.5, color: "var(--fg-0)", fontWeight: 500 }}>{c.name}</span>
                </div>
                <div style={{ fontSize: 10.5, color: "var(--fg-3)", marginLeft: 15 }}>{c.state}</div>
              </div>
            );
          })}
        </aside>
        {/* Detail */}
        <main style={{ padding: "24px 32px", overflow: "auto", display: "flex", flexDirection: "column", gap: 22 }}>
          <div>
            <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
              <div style={{ fontSize: 20, fontWeight: 600, color: "var(--fg-0)", letterSpacing: "-0.015em" }}>{selected.name}</div>
              <span style={{
                fontFamily: "var(--font-mono)", fontSize: 9.5,
                padding: "2px 8px", borderRadius: 999,
                background: "rgba(34,197,94,0.10)", border: "1px solid rgba(34,197,94,0.22)",
                color: "var(--success-400)", letterSpacing: "0.06em", textTransform: "uppercase",
              }}>Connected</span>
            </div>
            <div style={{ fontSize: 12.5, color: "var(--fg-2)", marginTop: 6, maxWidth: 460, lineHeight: 1.55 }}>{selected.desc}</div>
          </div>
          <div style={{ display: "flex", gap: 8 }}>
            <WinBtn tone="ghost" size="md" icon={<i className="fa-solid fa-arrow-rotate-right" style={{ fontSize: 11 }} />}>Test connection</WinBtn>
            <WinBtn tone="ghost" size="md" icon={<i className="fa-solid fa-arrow-up-right-from-square" style={{ fontSize: 10 }} />}>Open config in Finder</WinBtn>
            <div style={{ flex: 1 }} />
            <WinBtn tone="quiet" size="md">Disconnect</WinBtn>
          </div>
          <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 10 }}>
            {[
              { k: "Last ping",  v: selected.lastPing,    mono: true },
              { k: "Endpoint",   v: "localhost:8000/v1",  mono: true },
              { k: "Auth",       v: "sk-lthn-••••2qB7",   mono: true },
              { k: "Default model", v: "gemma-4-e2b",     mono: true },
            ].map((row) => (
              <div key={row.k} style={{
                padding: "10px 14px", borderRadius: 6,
                background: "rgba(255,255,255,0.025)",
                border: "1px solid rgba(255,255,255,0.05)",
              }}>
                <div style={{ fontSize: 10.5, color: "var(--fg-3)", letterSpacing: "0.04em", textTransform: "uppercase" }}>{row.k}</div>
                <div style={{ fontFamily: row.mono ? "var(--font-mono)" : "inherit", fontSize: 12.5, color: "var(--fg-0)", marginTop: 4 }}>{row.v}</div>
              </div>
            ))}
          </div>
          <div>
            <WinLabel>Config preview · what lthn writes</WinLabel>
            <div style={{
              marginTop: 8,
              background: "rgba(0,0,0,0.30)",
              border: "1px solid rgba(255,255,255,0.06)",
              borderRadius: 8,
              padding: "12px 14px",
              fontFamily: "var(--font-mono)", fontSize: 11.5, lineHeight: 1.6,
              color: "var(--fg-1)", whiteSpace: "pre",
            }}>
{`{
  "apiBase":  "http://localhost:8000/v1",
  "apiKey":   "sk-lthn-•••• (managed by lthn)",
  "model":    "gemma-4-e2b",
  "stream":   true,
  "managed":  true
}`}
            </div>
            <div style={{ marginTop: 10, fontSize: 11, color: "var(--fg-3)", lineHeight: 1.55 }}>
              Only the `apiBase`, `apiKey` and `model` keys are managed by lthn. Anything else you set in this file is left alone.
            </div>
          </div>
        </main>
      </div>
    </LthnWindow>
  );
}

/* ─────────────────────────────────────────────────────────────────
 * E3.2 · MCP tools window
 * ───────────────────────────────────────────────────────────────── */
function ToolsWindow({ w = 1040, h = 700 }) {
  const servers = [
    { id: "fs",       name: "filesystem",    tools: 4, on: true },
    { id: "git",      name: "git",           tools: 6, on: true },
    { id: "fetch",    name: "fetch",         tools: 2, on: true },
    { id: "sqlite",   name: "sqlite",        tools: 5, on: false },
    { id: "shell",    name: "shell",         tools: 1, on: false },
  ];
  const tools = [
    { server: "filesystem", name: "read_file",      desc: "Read a UTF-8 file at the given path", uses: 184, ms: 12, ok: 100 },
    { server: "filesystem", name: "write_file",     desc: "Write content to a file, creating it if missing", uses: 62, ms: 18, ok: 100, sel: true },
    { server: "filesystem", name: "list_dir",       desc: "List entries in a directory", uses: 218, ms: 8, ok: 100 },
    { server: "filesystem", name: "search_text",    desc: "Regex search across files", uses: 44, ms: 84, ok: 97.7 },
    { server: "git",        name: "status",         desc: "Show working-tree status", uses: 92, ms: 22, ok: 100 },
    { server: "git",        name: "diff",           desc: "Show diff for a path or commit range", uses: 48, ms: 38, ok: 100 },
  ];
  const sel = tools[1];
  return (
    <LthnWindow title="Tools · MCP" subtitle="2 servers · 12 tools · 648 calls today" width={w} height={h}
      toolbar={<>
        <WinBtn tone="ghost" size="sm" icon={<i className="fa-solid fa-plus" style={{ fontSize: 10 }} />}>Add server</WinBtn>
        <WinBtn tone="ghost" size="sm" icon={<i className="fa-solid fa-arrows-rotate" style={{ fontSize: 10 }} />}>Reload</WinBtn>
        <div style={{ flex: 1 }} />
        <span style={{ fontFamily: "var(--font-mono)", fontSize: 10.5, color: "var(--fg-3)" }}>
          tool-use availability depends on model · current model: gemma-4-e2b · ✓ supports tools
        </span>
      </>}
      footer={<>~/.lthn/mcp.json · 5 servers configured · 3 enabled · 648 calls today · 99.4 % ok</>}
    >
      <div style={{ flex: 1, display: "grid", gridTemplateColumns: "240px 1fr 320px", minHeight: 0 }}>
        {/* Servers/tools rail */}
        <aside style={{
          background: "rgba(0,0,0,0.18)",
          borderRight: "1px solid rgba(255,255,255,0.05)",
          padding: "12px 10px",
          overflow: "auto",
          display: "flex", flexDirection: "column", gap: 12,
        }}>
          {servers.map((s) => (
            <div key={s.id}>
              <div style={{
                display: "flex", alignItems: "center", gap: 8,
                padding: "4px 8px",
                fontSize: 11.5, color: "var(--fg-0)", fontWeight: 500,
              }}>
                <span style={{ width: 6, height: 6, borderRadius: "50%", background: s.on ? "var(--success-400)" : "var(--fg-3)" }} />
                <span>{s.name}</span>
                <span style={{ marginLeft: "auto", fontFamily: "var(--font-mono)", fontSize: 9.5, color: "var(--fg-3)" }}>{s.tools}</span>
                <SettingsToggle value={s.on} />
              </div>
              {s.on && (
                <div style={{ marginTop: 4, display: "flex", flexDirection: "column" }}>
                  {tools.filter((t) => t.server === s.name).map((t) => (
                    <div key={t.name} style={{
                      padding: "5px 14px 5px 22px",
                      fontFamily: "var(--font-mono)", fontSize: 11,
                      borderRadius: 4,
                      background: t.sel ? "rgba(255,255,255,0.06)" : "transparent",
                      color: t.sel ? "var(--fg-0)" : "var(--fg-2)",
                      cursor: "pointer",
                    }}>{t.name}</div>
                  ))}
                </div>
              )}
            </div>
          ))}
        </aside>

        {/* Schema + recent calls */}
        <main style={{ padding: "22px 26px", overflow: "auto", display: "flex", flexDirection: "column", gap: 18 }}>
          <div>
            <div style={{ display: "flex", alignItems: "baseline", gap: 10 }}>
              <span style={{ fontFamily: "var(--font-mono)", fontSize: 18, color: "var(--fg-0)", letterSpacing: "-0.005em" }}>filesystem.write_file</span>
              <span style={{ fontSize: 11, color: "var(--fg-3)" }}>· enabled</span>
            </div>
            <div style={{ fontSize: 12.5, color: "var(--fg-2)", marginTop: 5, lineHeight: 1.55 }}>{sel.desc}</div>
          </div>
          <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: 8 }}>
            {[
              { k: "Calls today",     v: sel.uses },
              { k: "Avg latency",     v: sel.ms + " ms" },
              { k: "Success rate",    v: sel.ok + " %" },
            ].map((m) => (
              <div key={m.k} style={{
                padding: "10px 14px", borderRadius: 6,
                background: "rgba(255,255,255,0.025)",
                border: "1px solid rgba(255,255,255,0.05)",
              }}>
                <div style={{ fontSize: 10.5, color: "var(--fg-3)", letterSpacing: "0.04em", textTransform: "uppercase" }}>{m.k}</div>
                <div style={{ fontFamily: "var(--font-mono)", fontSize: 18, color: "var(--fg-0)", marginTop: 4 }}>{m.v}</div>
              </div>
            ))}
          </div>
          <div>
            <WinLabel>Schema</WinLabel>
            <div style={{
              marginTop: 8,
              background: "rgba(0,0,0,0.30)",
              border: "1px solid rgba(255,255,255,0.06)",
              borderRadius: 8, padding: "12px 14px",
              fontFamily: "var(--font-mono)", fontSize: 11.5, lineHeight: 1.6,
              color: "var(--fg-1)", whiteSpace: "pre",
            }}>
{`{
  "name": "write_file",
  "description": "Write content to a file...",
  "parameters": {
    "path":     { "type": "string",  "required": true },
    "content":  { "type": "string",  "required": true },
    "encoding": { "type": "string",  "default":  "utf-8" },
    "create":   { "type": "boolean", "default":  true }
  }
}`}
            </div>
          </div>
          <div>
            <WinLabel>Recent calls</WinLabel>
            <div style={{
              marginTop: 8,
              background: "rgba(255,255,255,0.025)",
              border: "1px solid rgba(255,255,255,0.05)",
              borderRadius: 8,
              fontFamily: "var(--font-mono)", fontSize: 11,
            }}>
              {[
                { t: "14:32:21", p: '{ "path": "./notes/draft.md", "content": "..." }', ms: 14, ok: true },
                { t: "13:18:04", p: '{ "path": "./tmp/out.json", "content": "..." }', ms: 11, ok: true },
                { t: "12:08:42", p: '{ "path": "./.cache/lock", "create": false }',    ms: 0,  ok: false },
              ].map((c, i) => (
                <div key={i} style={{
                  display: "grid", gridTemplateColumns: "70px 1fr 50px 18px",
                  padding: "8px 14px", gap: 10,
                  borderBottom: i < 2 ? "1px solid rgba(255,255,255,0.04)" : "none",
                  alignItems: "center",
                }}>
                  <span style={{ color: "var(--fg-3)" }}>{c.t}</span>
                  <span style={{ color: "var(--fg-1)", whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>{c.p}</span>
                  <span style={{ color: c.ok ? "var(--fg-1)" : "var(--err-400)", textAlign: "right" }}>{c.ms} ms</span>
                  <i className={`fa-solid ${c.ok ? "fa-check" : "fa-xmark"}`}
                     style={{ fontSize: 10, color: c.ok ? "var(--success-400)" : "var(--err-400)" }} />
                </div>
              ))}
            </div>
          </div>
        </main>

        {/* Try-it */}
        <aside style={{
          background: "rgba(0,0,0,0.18)",
          borderLeft: "1px solid rgba(255,255,255,0.05)",
          padding: 18, overflow: "auto",
          display: "flex", flexDirection: "column", gap: 12,
        }}>
          <WinLabel>Try it · craft a test call</WinLabel>
          <div style={{
            background: "rgba(0,0,0,0.30)",
            border: "1px solid rgba(255,255,255,0.06)",
            borderRadius: 6,
            padding: 10,
            fontFamily: "var(--font-mono)", fontSize: 11.5, lineHeight: 1.6,
            color: "var(--fg-1)", whiteSpace: "pre", minHeight: 110,
          }}>
{`{
  "path":    "./scratch/hello.txt",
  "content": "hello, world\\n"
}`}
          </div>
          <WinBtn tone="primary" size="md" icon={<i className="fa-solid fa-play" style={{ fontSize: 10 }} />}>
            Invoke
          </WinBtn>
          <div style={{
            padding: "10px 12px", borderRadius: 6,
            background: "rgba(34,197,94,0.06)",
            border: "1px solid rgba(34,197,94,0.18)",
            fontSize: 11.5, color: "var(--fg-1)", lineHeight: 1.55,
          }}>
            <div style={{ display: "flex", alignItems: "center", gap: 6, marginBottom: 6 }}>
              <i className="fa-solid fa-check" style={{ color: "var(--success-400)", fontSize: 10 }} />
              <span style={{ color: "var(--success-400)", fontFamily: "var(--font-mono)", fontSize: 10, letterSpacing: "0.06em" }}>OK · 14 ms</span>
            </div>
            <div style={{ fontFamily: "var(--font-mono)", color: "var(--fg-2)", fontSize: 11 }}>
              wrote 13 bytes to ./scratch/hello.txt
            </div>
          </div>
          <div style={{ fontSize: 10.5, color: "var(--fg-3)", lineHeight: 1.55 }}>
            Test calls bypass the model — useful for sanity-checking a server before plumbing it into a tool-using chat.
          </div>
        </aside>
      </div>
    </LthnWindow>
  );
}

/* ─────────────────────────────────────────────────────────────────
 * E4 · Future-expansion concept sketches
 * ───────────────────────────────────────────────────────────────── */

/* E4.1 — Network / peers (v0.7+ LetherNet) */
function NetworkWindow({ w = 1080, h = 720 }) {
  // peers in a stylised layout — radial graph
  const peers = [
    { id: "self",  name: "this Mac · M3 Pro",   role: "you",        layers: "embeddings · 0-3", lat: "—",     x: 0.50, y: 0.50 },
    { id: "p1",    name: "tobias-m4 · M4 Max",  role: "peer",       layers: "attention · 4-15", lat: "8 ms",  x: 0.78, y: 0.30 },
    { id: "p2",    name: "vault-7950x · RTX",   role: "peer",       layers: "FFN · 16-31",      lat: "14 ms", x: 0.82, y: 0.62 },
    { id: "p3",    name: "homeserver · 7900",   role: "peer",       layers: "KV-cache",         lat: "11 ms", x: 0.22, y: 0.34 },
    { id: "p4",    name: "ana-air · M2",        role: "peer-idle",  layers: "—",                lat: "42 ms", x: 0.20, y: 0.68 },
  ];
  return (
    <LthnWindow title="Network" subtitle="LetherNet · v0.7 preview" width={w} height={h}
      toolbar={<>
        <WinBtn tone="primary" size="sm" active>This session</WinBtn>
        <WinBtn tone="ghost" size="sm">Available peers</WinBtn>
        <WinBtn tone="ghost" size="sm">Ledger</WinBtn>
        <div style={{ flex: 1 }} />
        <span style={{
          fontFamily: "var(--font-mono)", fontSize: 9.5,
          padding: "2px 8px", borderRadius: 999,
          background: "rgba(245,158,11,0.10)", border: "1px solid rgba(245,158,11,0.22)",
          color: "var(--warning-400)", letterSpacing: "0.06em", textTransform: "uppercase",
        }}>Preview · v0.7</span>
      </>}
      footer={<>Disaggregated · 4 peers · session privacy-preserved · no PII shared · always opt-in</>}
    >
      <div style={{ flex: 1, display: "grid", gridTemplateColumns: "1fr 340px", minHeight: 0 }}>
        {/* Map */}
        <main style={{
          background: "radial-gradient(circle at 50% 50%, rgba(64,193,197,0.05) 0%, rgba(11,16,22,0) 60%), var(--surf-0)",
          position: "relative", minHeight: 0, overflow: "hidden",
        }}>
          <svg viewBox="0 0 1000 600" width="100%" height="100%" preserveAspectRatio="xMidYMid meet">
            {/* Concentric rings */}
            {[80, 160, 240, 320].map((r) => (
              <circle key={r} cx="500" cy="300" r={r} fill="none" stroke="rgba(64,193,197,0.06)" strokeDasharray="2 4" />
            ))}
            {/* Edges */}
            {peers.slice(1).map((p) => (
              <line key={p.id} x1="500" y1="300"
                x2={p.x * 1000} y2={p.y * 600}
                stroke={p.role === "peer-idle" ? "rgba(255,255,255,0.10)" : "rgba(64,193,197,0.30)"}
                strokeWidth={p.role === "peer-idle" ? 1 : 1.6}
                strokeDasharray={p.role === "peer-idle" ? "3 3" : "0"} />
            ))}
            {/* Animated packets */}
            {peers.slice(1, 4).map((p, i) => {
              const x1 = 500, y1 = 300, x2 = p.x * 1000, y2 = p.y * 600;
              const mx = (x1 + x2) / 2 + (i - 1) * 12;
              const my = (y1 + y2) / 2 + (i - 1) * 8;
              return (
                <circle key={p.id} cx={mx} cy={my} r="3" fill="var(--brand-400)">
                  <animate attributeName="cx" values={`${x1};${x2};${x1}`} dur="3.2s" repeatCount="indefinite" begin={`${i * 0.4}s`} />
                  <animate attributeName="cy" values={`${y1};${y2};${y1}`} dur="3.2s" repeatCount="indefinite" begin={`${i * 0.4}s`} />
                  <animate attributeName="opacity" values="0;1;1;0" dur="3.2s" repeatCount="indefinite" begin={`${i * 0.4}s`} />
                </circle>
              );
            })}
            {/* Peer nodes */}
            {peers.map((p) => {
              const cx = p.x * 1000, cy = p.y * 600;
              const isSelf = p.role === "you";
              const idle = p.role === "peer-idle";
              return (
                <g key={p.id} transform={`translate(${cx}, ${cy})`}>
                  {isSelf && <circle r="34" fill="rgba(64,193,197,0.10)" />}
                  <circle r={isSelf ? 22 : 16} fill={idle ? "rgba(255,255,255,0.05)" : isSelf ? "var(--brand-500)" : "rgba(64,193,197,0.10)"}
                    stroke={idle ? "rgba(255,255,255,0.15)" : "var(--brand-400)"} strokeWidth={isSelf ? 0 : 1.5} />
                  {isSelf && <text y="6" fill="#fff" fontSize="14" textAnchor="middle" fontFamily="ui-monospace, monospace">λ</text>}
                  <text y={isSelf ? 50 : 36} fill="rgba(255,255,255,0.85)" fontSize="11" textAnchor="middle" fontFamily="ui-monospace, monospace">{p.name}</text>
                  <text y={isSelf ? 64 : 50} fill="rgba(255,255,255,0.40)" fontSize="9.5" textAnchor="middle" fontFamily="ui-monospace, monospace">{p.layers}</text>
                  {!isSelf && !idle && (
                    <text y="-22" fill="var(--brand-300)" fontSize="9" textAnchor="middle" fontFamily="ui-monospace, monospace">{p.lat}</text>
                  )}
                </g>
              );
            })}
          </svg>
          <div style={{
            position: "absolute", top: 14, left: 16,
            fontFamily: "var(--font-mono)", fontSize: 10.5, color: "var(--fg-3)", letterSpacing: "0.06em",
          }}>
            session · sora-1 · 142 tokens served · 14 ms median round-trip
          </div>
        </main>

        {/* Right rail — session detail + ledger preview */}
        <aside style={{
          background: "rgba(0,0,0,0.18)",
          borderLeft: "1px solid rgba(255,255,255,0.05)",
          padding: 20, overflow: "auto",
          display: "flex", flexDirection: "column", gap: 16,
        }}>
          <div>
            <WinLabel>Active session</WinLabel>
            <div style={{ fontFamily: "var(--font-mono)", fontSize: 13, color: "var(--fg-0)", marginTop: 6 }}>sora-1 · 70B</div>
            <div style={{ fontSize: 11, color: "var(--fg-3)", marginTop: 3 }}>
              Split across 4 machines · 1 of which is yours
            </div>
          </div>
          <div style={{ display: "flex", flexDirection: "column", gap: 6, fontSize: 11.5 }}>
            <RailRow k="You serve"      v="Embeddings · L0–3" />
            <RailRow k="Peers serve"    v="Attention · FFN · KV" />
            <RailRow k="Median latency" v="14 ms" />
            <RailRow k="Privacy"        v="prompts split client-side" />
            <RailRow k="Ledger"         v="0.0142 LTHN earned" />
          </div>
          <div style={{
            padding: 12, borderRadius: 6,
            background: "rgba(64,193,197,0.06)",
            border: "1px solid rgba(64,193,197,0.18)",
            fontSize: 11.5, color: "var(--fg-1)", lineHeight: 1.55,
          }}>
            <div style={{ display: "flex", alignItems: "center", gap: 6, marginBottom: 6 }}>
              <i className="fa-solid fa-shield-halved" style={{ color: "var(--brand-300)", fontSize: 11 }} />
              <span style={{ color: "var(--brand-300)", fontFamily: "var(--font-mono)", fontSize: 10, letterSpacing: "0.06em", textTransform: "uppercase" }}>Privacy</span>
            </div>
            Peers see model layers, not your prompts. Prompts are split + masked client-side before any layer leaves this Mac.
          </div>
          <div style={{ fontSize: 11, color: "var(--fg-3)", lineHeight: 1.55, fontStyle: "italic" }}>
            "Sovereign first. Federated when you opt in. Never the other way round."
          </div>
          <WinBtn tone="quiet" size="md" style={{ justifyContent: "center" }}>Leave session</WinBtn>
        </aside>
      </div>
    </LthnWindow>
  );
}

/* E4.2 — Distillation / fine-tune window */
function DistillationWindow({ w = 1100, h = 740 }) {
  // loss curve
  const loss = Array.from({ length: 40 }, (_, i) => 2.4 * Math.exp(-i * 0.06) + 0.4 + (Math.random() - 0.5) * 0.08);
  const cw = 740, ch = 220, pad = { l: 40, r: 14, t: 12, b: 24 };
  return (
    <LthnWindow title="Fine-tune" subtitle="LoRA · SFT · distill · merge" width={w} height={h}
      toolbar={<>
        {[
          { id: "1", label: "Base model" },
          { id: "2", label: "Dataset" },
          { id: "3", label: "Config" },
          { id: "4", label: "Run" },
          { id: "5", label: "Publish" },
        ].map((s, i) => (
          <div key={s.id} style={{ display: "flex", alignItems: "center", gap: 6 }}>
            <div style={{
              width: 18, height: 18, borderRadius: "50%",
              border: "1.5px solid " + (i < 3 ? "var(--brand-500)" : i === 3 ? "var(--brand-400)" : "rgba(255,255,255,0.12)"),
              background: i < 3 ? "var(--brand-500)" : "transparent",
              display: "flex", alignItems: "center", justifyContent: "center",
              fontSize: 10, fontWeight: 600,
              color: i < 3 ? "#fff" : i === 3 ? "var(--brand-300)" : "var(--fg-3)",
            }}>{i < 3 ? <i className="fa-solid fa-check" style={{ fontSize: 8 }} /> : s.id}</div>
            <span style={{ fontSize: 12, color: i <= 3 ? "var(--fg-0)" : "var(--fg-3)", fontWeight: i === 3 ? 500 : 400 }}>{s.label}</span>
            {i < 4 && <span style={{ width: 24, height: 1, background: "rgba(255,255,255,0.08)", margin: "0 8px" }} />}
          </div>
        ))}
        <div style={{ flex: 1 }} />
        <WinBtn tone="quiet" size="sm" icon={<i className="fa-solid fa-stop" style={{ fontSize: 9 }} />}>Stop</WinBtn>
      </>}
      footer={<>step 4 of 5 · running · epoch 2/3 · loss 0.84 · ETA 14 min · 9.8 W</>}
    >
      <div style={{ flex: 1, display: "grid", gridTemplateColumns: "300px 1fr 320px", minHeight: 0 }}>
        {/* Config rail */}
        <aside style={{
          background: "rgba(0,0,0,0.18)",
          borderRight: "1px solid rgba(255,255,255,0.05)",
          padding: 18, overflow: "auto",
          display: "flex", flexDirection: "column", gap: 16,
        }}>
          <div>
            <WinLabel>Recipe</WinLabel>
            <div style={{ marginTop: 8, display: "flex", flexDirection: "column", gap: 8, fontSize: 11.5 }}>
              <RailRow k="Method"   v="LoRA · AdamW" />
              <RailRow k="Rank"     v="16" />
              <RailRow k="Alpha"    v="32" />
              <RailRow k="Dropout"  v="0.05" />
              <RailRow k="LR"       v="1e-4 · cosine" />
              <RailRow k="Batch"    v="8 · grad-accum 4" />
              <RailRow k="Epochs"   v="3" />
              <RailRow k="Targets"  v="q_proj · v_proj · o_proj" />
            </div>
          </div>
          <div style={{ height: 1, background: "rgba(255,255,255,0.05)" }} />
          <div>
            <WinLabel>Base + dataset</WinLabel>
            <div style={{ marginTop: 8, display: "flex", flexDirection: "column", gap: 8, fontSize: 11.5 }}>
              <RailRow k="Base"     v="gemma-4-e2b" />
              <RailRow k="Dataset"  v="lthn-helpcenter-v3" />
              <RailRow k="Samples"  v="4,820" />
              <RailRow k="Split"    v="train 4.5k · eval 320" />
              <RailRow k="Format"   v="ChatML · {prompt, completion}" />
            </div>
          </div>
        </aside>
        {/* Run surface */}
        <main style={{ padding: "20px 26px", overflow: "auto", display: "flex", flexDirection: "column", gap: 18 }}>
          <div style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: 8 }}>
            {[
              { k: "Epoch",   v: "2 / 3",       sub: "step 184 / 270" },
              { k: "Loss",    v: "0.84",        sub: "↓ from 2.31" },
              { k: "tok/s",   v: "1,142",       sub: "training throughput" },
              { k: "Watts",   v: "9.8 W",       sub: "GPU + ANE" },
            ].map((m) => (
              <div key={m.k} style={{
                padding: "12px 14px", borderRadius: 8,
                background: "rgba(255,255,255,0.025)",
                border: "1px solid rgba(255,255,255,0.06)",
              }}>
                <div style={{ fontSize: 10.5, color: "var(--fg-3)", letterSpacing: "0.04em", textTransform: "uppercase" }}>{m.k}</div>
                <div style={{ fontFamily: "var(--font-mono)", fontSize: 22, color: "var(--fg-0)", marginTop: 4, letterSpacing: "-0.01em" }}>{m.v}</div>
                <div style={{ fontSize: 10.5, color: "var(--fg-3)", marginTop: 3 }}>{m.sub}</div>
              </div>
            ))}
          </div>
          <div style={{
            background: "rgba(0,0,0,0.20)",
            border: "1px solid rgba(255,255,255,0.05)",
            borderRadius: 8, padding: 12,
          }}>
            <WinLabel>Loss · steps 0 → 184</WinLabel>
            <svg viewBox={`0 0 ${cw} ${ch}`} width="100%" height={ch} preserveAspectRatio="none" style={{ marginTop: 4 }}>
              {[0, 0.6, 1.2, 1.8, 2.4].map((y) => {
                const yy = pad.t + (1 - y / 2.4) * (ch - pad.t - pad.b);
                return (<g key={y}>
                  <line x1={pad.l} x2={cw - pad.r} y1={yy} y2={yy} stroke="rgba(255,255,255,0.05)" />
                  <text x={pad.l - 6} y={yy + 3} fill="rgba(255,255,255,0.40)" fontSize="9.5" textAnchor="end" fontFamily="ui-monospace, monospace">{y.toFixed(1)}</text>
                </g>);
              })}
              <path d={"M " + loss.map((v, i) => `${pad.l + (i / (loss.length - 1)) * (cw - pad.l - pad.r)} ${pad.t + (1 - v / 2.4) * (ch - pad.t - pad.b)}`).join(" L ")}
                stroke="var(--brand-400)" strokeWidth="1.6" fill="none" />
              <line x1={pad.l + 0.68 * (cw - pad.l - pad.r)} x2={pad.l + 0.68 * (cw - pad.l - pad.r)}
                y1={pad.t} y2={ch - pad.b} stroke="var(--warning-400)" strokeDasharray="3 3" />
              <text x={pad.l + 0.68 * (cw - pad.l - pad.r) + 4} y={pad.t + 12}
                fill="var(--warning-400)" fontSize="9.5" fontFamily="ui-monospace, monospace">epoch 2 begins</text>
            </svg>
          </div>
          <div>
            <WinLabel>Sample · eval prompt #142</WinLabel>
            <div style={{
              marginTop: 8, display: "grid", gridTemplateColumns: "1fr 1fr", gap: 8,
            }}>
              {[
                { who: "base · pre-tune",  text: "Sure! Here are some general tips that may help you set up a Lethean instance, though I'm not certain about the specifics…", tone: "var(--fg-3)" },
                { who: "ours · step 184",  text: "Add `LTHN_HOME=~/.lthn` to your shell, then `lthn runner start --model gemma-4-e2b`. The tray icon should appear within a few seconds.", tone: "var(--fg-1)" },
              ].map((s) => (
                <div key={s.who} style={{
                  padding: "10px 12px", borderRadius: 6,
                  background: "rgba(255,255,255,0.025)",
                  border: "1px solid rgba(255,255,255,0.05)",
                }}>
                  <div style={{ fontFamily: "var(--font-mono)", fontSize: 9.5, color: "var(--fg-3)", letterSpacing: "0.06em", textTransform: "uppercase", marginBottom: 6 }}>{s.who}</div>
                  <div style={{ fontSize: 11.5, color: s.tone, lineHeight: 1.55 }}>{s.text}</div>
                </div>
              ))}
            </div>
          </div>
        </main>
        {/* Output rail */}
        <aside style={{
          background: "rgba(0,0,0,0.18)",
          borderLeft: "1px solid rgba(255,255,255,0.05)",
          padding: 18, overflow: "auto",
          display: "flex", flexDirection: "column", gap: 14,
        }}>
          <div>
            <WinLabel>Output adapter</WinLabel>
            <div style={{ fontFamily: "var(--font-mono)", fontSize: 12, color: "var(--fg-0)", marginTop: 6, letterSpacing: "-0.005em" }}>
              gemma-4-e2b-helpcenter-lora
            </div>
            <div style={{ fontSize: 11, color: "var(--fg-3)", marginTop: 3 }}>~/.lthn/adapters/ · 42 MB</div>
          </div>
          <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
            <WinBtn tone="primary" size="md" style={{ justifyContent: "center" }}
              icon={<i className="fa-regular fa-comment" />}>Test in chat</WinBtn>
            <WinBtn tone="ghost" size="md" style={{ justifyContent: "center" }}
              icon={<i className="fa-solid fa-code-merge" style={{ fontSize: 11 }} />}>Merge into base</WinBtn>
            <WinBtn tone="ghost" size="md" style={{ justifyContent: "center" }}
              icon={<i className="fa-solid fa-cloud-arrow-up" style={{ fontSize: 11 }} />}>Push to HuggingFace</WinBtn>
          </div>
          <div style={{ height: 1, background: "rgba(255,255,255,0.05)" }} />
          <div>
            <WinLabel>System</WinLabel>
            <div style={{ marginTop: 8, display: "flex", flexDirection: "column", gap: 6, fontSize: 11 }}>
              <RailRow k="Backend"   v="go-mlx · Metal" />
              <RailRow k="GPU mem"   v="13.2 / 36 GB" />
              <RailRow k="Disk i/o"  v="22 MB/s" />
              <RailRow k="ETA"       v="14 min" />
            </div>
          </div>
          <div style={{ fontSize: 11, color: "var(--fg-3)", fontStyle: "italic", lineHeight: 1.55 }}>
            Training runs locally. The dataset stays on this Mac. The adapter is yours.
          </div>
        </aside>
      </div>
    </LthnWindow>
  );
}

/* E4.3 — Fleet / workspace */
function FleetWindow({ w = 1080, h = 700 }) {
  const machines = [
    { id: "this-mac",      name: "this Mac",          arch: "M3 Pro · 36 GB",  status: "online · loaded",   model: "gemma-4-e2b",   load: 32, tps: "47.2", you: true },
    { id: "studio",        name: "vault · Studio M2", arch: "M2 Ultra · 192 GB", status: "online · idle",    model: "gemma-3-27b",   load: 4,  tps: "0" },
    { id: "ws",            name: "shop · 7950X · RTX 4090", arch: "x86 · 24 GB VRAM", status: "online · loaded", model: "llama-3.3-70b", load: 78, tps: "11.4" },
    { id: "homeserver",    name: "homeserver · 7900X", arch: "x86 · 96 GB",     status: "online · idle",    model: "—",             load: 2,  tps: "0" },
    { id: "ana-air",       name: "ana-air · M2",      arch: "M2 · 16 GB",      status: "offline",          model: "—",             load: 0,  tps: "—",  offline: true },
  ];
  const queue = [
    { id: "q1", who: "claude-code",  model: "gemma-4-e2b",  route: "this Mac",  state: "running",  start: "14:32:14", elapsed: "3.2 s" },
    { id: "q2", who: "raycast",      model: "gemma-4-e2b",  route: "this Mac",  state: "queued",   start: "—",        elapsed: "—" },
    { id: "q3", who: "opencode",     model: "llama-3.3-70b",route: "shop",      state: "running",  start: "14:32:08", elapsed: "9.1 s" },
  ];
  return (
    <LthnWindow title="Fleet" subtitle="multi-machine · v1.0 preview" width={w} height={h}
      toolbar={<>
        <WinBtn tone="primary" size="sm" active>Machines</WinBtn>
        <WinBtn tone="ghost" size="sm">Routing rules</WinBtn>
        <WinBtn tone="ghost" size="sm">Snapshots</WinBtn>
        <div style={{ flex: 1 }} />
        <span style={{
          fontFamily: "var(--font-mono)", fontSize: 9.5,
          padding: "2px 8px", borderRadius: 999,
          background: "rgba(245,158,11,0.10)", border: "1px solid rgba(245,158,11,0.22)",
          color: "var(--warning-400)", letterSpacing: "0.06em", textTransform: "uppercase",
        }}>Preview · v1.0</span>
      </>}
      footer={<>4 of 5 online · routing latency-aware · ⌘R reroute · ⌘S snapshot</>}
    >
      <div style={{ flex: 1, display: "flex", flexDirection: "column", minHeight: 0 }}>
        <div style={{ padding: "16px 22px 8px" }}>
          <WinLabel>Machines · drag-reorder to set route priority</WinLabel>
        </div>
        <div style={{ padding: "0 22px", display: "flex", flexDirection: "column", gap: 8 }}>
          {machines.map((m) => (
            <div key={m.id} style={{
              display: "grid", gridTemplateColumns: "16px 1.3fr 1.2fr 1.2fr 1fr 0.8fr 60px",
              gap: 14,
              padding: "12px 16px",
              borderRadius: 8,
              background: m.offline ? "rgba(255,255,255,0.015)" : m.you ? "rgba(64,193,197,0.06)" : "rgba(255,255,255,0.03)",
              border: m.you ? "1px solid rgba(64,193,197,0.22)" : "1px solid rgba(255,255,255,0.05)",
              opacity: m.offline ? 0.55 : 1,
              alignItems: "center",
            }}>
              <i className="fa-solid fa-grip-vertical" style={{ fontSize: 11, color: "var(--fg-3)" }} />
              <div>
                <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                  <span style={{ width: 7, height: 7, borderRadius: "50%",
                    background: m.offline ? "var(--fg-3)" : "var(--success-400)",
                    boxShadow: !m.offline ? "0 0 4px var(--success-400)" : "none" }} />
                  <span style={{ fontSize: 13, color: "var(--fg-0)", fontWeight: 500 }}>{m.name}</span>
                  {m.you && <span style={{
                    fontFamily: "var(--font-mono)", fontSize: 9, padding: "1px 6px",
                    borderRadius: 999, background: "rgba(64,193,197,0.12)",
                    border: "1px solid rgba(64,193,197,0.22)",
                    color: "var(--brand-300)", letterSpacing: "0.06em", textTransform: "uppercase",
                  }}>You</span>}
                </div>
                <div style={{ fontFamily: "var(--font-mono)", fontSize: 10.5, color: "var(--fg-3)", marginTop: 3 }}>{m.arch}</div>
              </div>
              <div style={{ fontSize: 11.5, color: m.offline ? "var(--fg-3)" : "var(--fg-2)" }}>{m.status}</div>
              <div style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--fg-1)" }}>{m.model}</div>
              {/* Load bar */}
              <div>
                <div style={{ display: "flex", justifyContent: "space-between", fontSize: 10, color: "var(--fg-3)", marginBottom: 3 }}>
                  <span>load</span>
                  <span style={{ fontFamily: "var(--font-mono)", color: "var(--fg-1)" }}>{m.load}%</span>
                </div>
                <div style={{ height: 4, background: "rgba(255,255,255,0.06)", borderRadius: 2, overflow: "hidden" }}>
                  <div style={{ width: `${m.load}%`, height: "100%", background: m.load > 70 ? "var(--warning-400)" : "var(--brand-400)" }} />
                </div>
              </div>
              <div style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--fg-1)", textAlign: "right" }}>{m.tps} tok/s</div>
              <WinBtn tone="quiet" size="sm" icon={<i className="fa-solid fa-ellipsis" />} />
            </div>
          ))}
        </div>

        <div style={{ padding: "20px 22px 8px" }}>
          <WinLabel>Live queue</WinLabel>
        </div>
        <div style={{
          margin: "0 22px 22px",
          background: "rgba(255,255,255,0.025)",
          border: "1px solid rgba(255,255,255,0.06)",
          borderRadius: 8,
          fontFamily: "var(--font-mono)", fontSize: 11.5,
        }}>
          <div style={{
            display: "grid", gridTemplateColumns: "100px 1fr 1fr 1fr 0.8fr 0.8fr",
            padding: "10px 14px",
            borderBottom: "1px solid rgba(255,255,255,0.05)",
            color: "var(--fg-3)", fontSize: 10, letterSpacing: "0.04em", textTransform: "uppercase",
          }}>
            <span>State</span><span>Caller</span><span>Model</span><span>Routed to</span><span>Started</span><span>Elapsed</span>
          </div>
          {queue.map((q) => (
            <div key={q.id} style={{
              display: "grid", gridTemplateColumns: "100px 1fr 1fr 1fr 0.8fr 0.8fr",
              padding: "10px 14px",
              borderBottom: "1px solid rgba(255,255,255,0.04)",
              alignItems: "center",
            }}>
              <span style={{
                fontSize: 9.5, padding: "2px 8px", borderRadius: 999,
                background: q.state === "running" ? "rgba(34,197,94,0.10)" : "rgba(255,255,255,0.05)",
                border: q.state === "running" ? "1px solid rgba(34,197,94,0.22)" : "1px solid rgba(255,255,255,0.07)",
                color: q.state === "running" ? "var(--success-400)" : "var(--fg-2)",
                letterSpacing: "0.06em", textTransform: "uppercase",
                width: "fit-content",
              }}>{q.state}</span>
              <span style={{ color: "var(--fg-1)" }}>{q.who}</span>
              <span style={{ color: "var(--fg-0)" }}>{q.model}</span>
              <span style={{ color: "var(--brand-300)" }}>{q.route}</span>
              <span style={{ color: "var(--fg-2)" }}>{q.start}</span>
              <span style={{ color: "var(--fg-1)" }}>{q.elapsed}</span>
            </div>
          ))}
        </div>
      </div>
    </LthnWindow>
  );
}

Object.assign(window, {
  BenchmarkWindow, LogsWindow, TelemetryWindow,
  IntegrationsWindow, ToolsWindow,
  NetworkWindow, DistillationWindow, FleetWindow,
});
