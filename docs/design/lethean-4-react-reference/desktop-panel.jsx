/* desktop-panel.jsx
 * Lethean Desktop — P0 tray popover panel + supporting bits.
 * 400×560 popover, five sections (header / stats / prompt+output / footer).
 * Parameterised by `state` so we can render all 5 variants on one canvas.
 */

/* ── Glyph ───────────────────────────────────────────────────────────
 * Lethean Corinthian helmet — the official brand mark. Front profile,
 * crest with white parting line, T-shaped eye/nose slit. Rendered as a
 * CSS-mask of the supplied PNG so it inherits `color` and stays
 * template-image-safe on any background (light menu bar, dark menu
 * bar, panel header, all surfaces).
 * Active state adds a small teal runner dot at the upper-right of the
 * crest — the only chrome added by us; the mark itself is untouched.
 * Asset source: brand guidelines, hoplite_black/white/gradient.
 * ─────────────────────────────────────────────────────────────────── */
function LthnGlyph({ size = 16, color = "currentColor", active = false, accent }) {
  const acc = accent || "var(--brand-400)";
  // Asset aspect ratio: 1500 / 2123 ≈ 0.7065. size is the bounding box
  // height; the helmet renders centred with auto width.
  const w = Math.round(size * 0.7065);
  // Dot scales with size, pinned to the top-right of the bounding box,
  // tucked slightly into the silhouette so it reads as part of the mark.
  const dotR = Math.max(1.4, size * 0.095);
  const dotCx = size - dotR * 0.6;
  const dotCy = dotR * 0.6;
  return (
    <span style={{
      position: "relative",
      display: "inline-block",
      width: size,
      height: size,
      lineHeight: 0,
    }} aria-hidden="true">
      <span style={{
        position: "absolute",
        top: 0,
        left: (size - w) / 2,
        width: w,
        height: size,
        background: color,
        WebkitMaskImage: "url(assets/hoplite_black.png)",
        maskImage: "url(assets/hoplite_black.png)",
        WebkitMaskRepeat: "no-repeat",
        maskRepeat: "no-repeat",
        WebkitMaskSize: "contain",
        maskSize: "contain",
        WebkitMaskPosition: "center",
        maskPosition: "center",
      }} />
      {active && (
        <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`} style={{ position: "absolute", inset: 0, overflow: "visible" }}>
          <circle cx={dotCx} cy={dotCy} r={dotR} fill={acc} />
        </svg>
      )}
    </span>
  );
}

/* "lthn" lowercase wordmark, mono-feeling, tight tracking */
function LthnWordmark({ size = 13, color = "var(--fg-0)" }) {
  return (
    <span style={{
      fontFamily: "var(--font-mono)",
      fontSize: size,
      fontWeight: 500,
      letterSpacing: "-0.01em",
      color,
      lineHeight: 1,
    }}>lthn</span>
  );
}

/* ── Status dot ──────────────────────────────────────────────────── */
function StatusDot({ tone = "ok", pulse = false }) {
  const tones = {
    idle:  "var(--fg-3)",
    warn:  "var(--warning-400)",
    ok:    "var(--success-400)",
    busy:  "var(--success-400)",
    error: "var(--danger-400)",
  };
  const c = tones[tone] || tones.idle;
  return (
    <span style={{ position: "relative", width: 7, height: 7, display: "inline-block" }}>
      <span style={{
        position: "absolute", inset: 0, borderRadius: "50%", background: c,
        boxShadow: tone === "busy" || pulse ? `0 0 0 0 ${c}` : "none",
        animation: tone === "busy" || pulse ? "lthn-pulse 1.6s ease-out infinite" : "none",
      }} />
      <span style={{
        position: "absolute", inset: 0, borderRadius: "50%",
        boxShadow: `0 0 6px ${c}`, opacity: tone === "idle" ? 0 : 0.5,
      }} />
    </span>
  );
}

/* ── Sparkline ──────────────────────────────────────────────────────
 * 60×20 inline tok/s history. Line + faint area fill, no glow on the
 * curve. Trailing data-point gets a 2px violet dot (the only thing
 * that pulses, so the eye lands on the live value).
 * ─────────────────────────────────────────────────────────────────── */
function Sparkline({ width = 60, height = 20, data, stroke, fill, pulseHead = true }) {
  // Default data — a plausible tok/s curve climbing then stabilising
  const series = data || [12, 18, 24, 31, 36, 39, 41, 44, 46, 47, 47, 47];
  const max = Math.max(...series);
  const min = Math.min(...series);
  const span = Math.max(1, max - min);
  const step = width / (series.length - 1);
  const pts = series.map((v, i) => {
    const x = i * step;
    const y = height - 2 - ((v - min) / span) * (height - 4);
    return [x, y];
  });
  const linePath = pts.map(([x, y], i) => `${i ? "L" : "M"} ${x.toFixed(2)} ${y.toFixed(2)}`).join(" ");
  const areaPath = `${linePath} L ${width} ${height} L 0 ${height} Z`;
  const [hx, hy] = pts[pts.length - 1];
  const s = stroke || "var(--brand-400)";
  const f = fill || "var(--brand-400)";
  return (
    <svg width={width} height={height} viewBox={`0 0 ${width} ${height}`} style={{ display: "block", overflow: "visible" }}>
      <path d={areaPath} fill={f} opacity="0.14" />
      <path d={linePath} stroke={s} strokeWidth="1.25" fill="none" strokeLinecap="round" strokeLinejoin="round" />
      <circle cx={hx} cy={hy} r="1.8" fill={s} />
      {pulseHead && (
        <circle cx={hx} cy={hy} r="1.8" fill={s} opacity="0.5">
          <animate attributeName="r" values="1.8;5.5;1.8" dur="1.6s" repeatCount="indefinite" />
          <animate attributeName="opacity" values="0.5;0;0.5" dur="1.6s" repeatCount="indefinite" />
        </circle>
      )}
    </svg>
  );
}

/* ── Panel sections ──────────────────────────────────────────────── */

function PanelHeader({ canStart, running, onAction }) {
  return (
    <div style={{
      height: 40, padding: "0 12px",
      display: "flex", alignItems: "center", justifyContent: "space-between",
      borderBottom: "1px solid var(--line-1)",
      flexShrink: 0,
    }}>
      <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
        <LthnGlyph size={16} color="var(--fg-1)" active={running} />
        <LthnWordmark size={13} />
        <span style={{
          marginLeft: 4, fontFamily: "var(--font-mono)", fontSize: 10.5,
          color: "var(--fg-3)", letterSpacing: "0.04em", textTransform: "uppercase",
        }}>Desktop</span>
      </div>
      <button style={{
        appearance: "none", border: "1px solid var(--line-2)",
        background: running ? "transparent" : "var(--ink-3)",
        color: running ? "var(--fg-1)" : "var(--fg-0)",
        height: 24, padding: "0 10px", borderRadius: 5,
        fontFamily: "var(--font-sans)", fontSize: 11.5, fontWeight: 500,
        letterSpacing: "0.01em", display: "flex", alignItems: "center", gap: 6,
        cursor: canStart ? "pointer" : "default",
        opacity: canStart ? 1 : 0.4,
      }}>
        {running ? (
          <><span style={{ width: 7, height: 7, background: "var(--fg-2)", borderRadius: 1 }} />Stop</>
        ) : (
          <><span style={{
            width: 0, height: 0, borderTop: "4px solid transparent",
            borderBottom: "4px solid transparent", borderLeft: "6px solid var(--brand-400)",
            marginRight: 1,
          }} />Start</>
        )}
      </button>
    </div>
  );
}

function StatRow({ label, value, mono = true, dim = false, suffix }) {
  return (
    <div style={{
      display: "flex", alignItems: "baseline", justifyContent: "space-between",
      padding: "0 12px", height: 24,
    }}>
      <span style={{
        fontFamily: "var(--font-mono)", fontSize: 10, fontWeight: 500,
        letterSpacing: "0.08em", textTransform: "uppercase",
        color: "var(--fg-3)",
      }}>{label}</span>
      <span style={{
        fontFamily: mono ? "var(--font-mono)" : "var(--font-sans)",
        fontSize: 12.5,
        color: dim ? "var(--fg-3)" : "var(--fg-0)",
        fontVariantNumeric: "tabular-nums",
        letterSpacing: "-0.005em",
      }}>
        {value}
        {suffix && <span style={{ color: "var(--fg-3)", marginLeft: 2 }}>{suffix}</span>}
      </span>
    </div>
  );
}

function StatsStrip({ stats }) {
  return (
    <div style={{
      paddingTop: 8, paddingBottom: 8,
      borderBottom: "1px solid var(--line-1)",
      flexShrink: 0,
    }}>
      <StatRow label="Model"      value={stats.model}  dim={stats.model === "—"} />
      <StatRow label="Tokens / s" value={stats.tps}    dim={stats.tps === "—"} />
      <StatRow label="Power"      value={stats.power}  dim={stats.power === "—"} />
      <StatRow label="Memory"     value={stats.memory} dim={stats.memory === "—"} />
    </div>
  );
}

function PromptArea({ state, prompt, output, showCursor }) {
  const disabled = state === "first-run" || state === "loading";
  const helper = {
    "first-run": "Drop a model into ~/.lthn/models/ or run lthn ai models pull <name>",
    "loading":   "Model loading. Prompt will unlock when ready.",
  }[state];

  return (
    <div style={{
      flex: 1, display: "flex", flexDirection: "column",
      padding: "10px 12px 8px", gap: 8, minHeight: 0,
    }}>
      {/* Error toast — only when state === error */}
      {state === "error" && (
        <div style={{
          background: "color-mix(in oklab, var(--danger-500) 14%, transparent)",
          border: "1px solid color-mix(in oklab, var(--danger-500) 35%, transparent)",
          borderRadius: 6, padding: "8px 10px",
          display: "flex", gap: 8, alignItems: "flex-start",
          fontFamily: "var(--font-sans)", fontSize: 11.5, lineHeight: 1.4,
          color: "var(--fg-1)",
        }}>
          <svg width="12" height="12" viewBox="0 0 12 12" style={{ flexShrink: 0, marginTop: 1 }}>
            <path d="M6 1 L11 10.5 L1 10.5 Z" fill="none" stroke="var(--danger-400)" strokeWidth="1.2" strokeLinejoin="round" />
            <rect x="5.4" y="4.5" width="1.2" height="3" fill="var(--danger-400)" />
            <rect x="5.4" y="8.2" width="1.2" height="1.2" fill="var(--danger-400)" />
          </svg>
          <div>
            <div style={{ color: "var(--fg-0)", fontWeight: 500, marginBottom: 2 }}>Out of memory</div>
            <div style={{ color: "var(--fg-2)" }}>The model needs 2.4 GB but only 1.8 GB is free. Close another app or pick a smaller model.</div>
          </div>
        </div>
      )}

      {/* Textarea */}
      <div style={{ position: "relative" }}>
        <div style={{
          background: "var(--ink-3)",
          border: `1px solid ${disabled ? "var(--line-1)" : "var(--line-2)"}`,
          borderRadius: 6,
          padding: "8px 10px",
          minHeight: 64,
          fontFamily: "var(--font-mono)",
          fontSize: 12.5, lineHeight: 1.45,
          color: disabled ? "var(--fg-3)" : "var(--fg-0)",
          opacity: disabled ? 0.55 : 1,
          whiteSpace: "pre-wrap",
        }}>
          {prompt || (
            <span style={{ color: "var(--fg-3)" }}>
              {disabled ? helper : "Ask Lethean…"}
            </span>
          )}
          {showCursor && !disabled && (
            <span style={{
              display: "inline-block", width: 6, height: 13,
              background: "var(--brand-400)", verticalAlign: "-2px",
              marginLeft: 1, animation: "lthn-cursor 1s steps(2) infinite",
            }} />
          )}
        </div>
      </div>

      {/* Generate button */}
      <div style={{ display: "flex", justifyContent: "flex-end", alignItems: "center", gap: 8 }}>
        {state === "ready" && (
          <span style={{
            fontFamily: "var(--font-mono)", fontSize: 10,
            color: "var(--fg-3)", letterSpacing: "0.04em",
          }}>⌘ ↩ to send</span>
        )}
        <button disabled={disabled} style={{
          appearance: "none",
          height: 26, padding: "0 12px", borderRadius: 5,
          background: disabled ? "var(--ink-3)" : "var(--brand-500)",
          color: disabled ? "var(--fg-3)" : "white",
          border: "1px solid " + (disabled ? "var(--line-1)" : "var(--brand-600)"),
          fontFamily: "var(--font-sans)", fontSize: 11.5, fontWeight: 500,
          cursor: disabled ? "default" : "pointer",
          display: "flex", alignItems: "center", gap: 6,
        }}>
          {state === "generating" ? (
            <>
              <span style={{ width: 6, height: 6, background: "white", borderRadius: 1 }} />
              Stop
            </>
          ) : (
            <>Generate</>
          )}
        </button>
      </div>

      {/* Output area */}
      {output && (
        <div style={{
          flex: 1, minHeight: 0, position: "relative",
          background: "var(--ink-3)",
          border: "1px solid var(--line-1)",
          borderRadius: 6,
          overflow: "hidden",
          display: "flex", flexDirection: "column",
        }}>
          {/* Tiny header strip with sparkline */}
          <div style={{
            display: "flex", justifyContent: "space-between", alignItems: "center",
            padding: "5px 8px 4px",
            borderBottom: "1px solid var(--line-1)",
            background: "var(--ink-1)",
            flexShrink: 0,
          }}>
            <span style={{
              fontFamily: "var(--font-mono)", fontSize: 9.5,
              color: "var(--fg-3)", letterSpacing: "0.08em",
              textTransform: "uppercase",
            }}>Output</span>
            <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
              {output.live && <Sparkline width={60} height={16} />}
              <span style={{
                fontFamily: "var(--font-mono)", fontSize: 9.5,
                color: "var(--fg-2)", fontVariantNumeric: "tabular-nums",
              }}>{output.live ? "47 t/s" : `${output.tokens} tokens`}</span>
            </div>
          </div>
          <div style={{
            flex: 1, padding: "8px 10px", overflow: "auto",
            fontFamily: "var(--font-mono)", fontSize: 12, lineHeight: 1.5,
            color: "var(--fg-1)",
          }}>
            <OutputBody body={output.body} live={output.live} />
          </div>
        </div>
      )}
    </div>
  );
}

function OutputBody({ body, live }) {
  // Render the body string with selection on a substring + live cursor at end
  return (
    <>
      <span>{body.before}</span>
      <span style={{
        background: "color-mix(in oklab, var(--brand-500) 35%, transparent)",
        color: "var(--fg-0)",
        borderRadius: 1,
      }}>{body.selection}</span>
      <span>{body.after}</span>
      {live && (
        <span style={{
          display: "inline-block", width: 6, height: 13,
          background: "var(--brand-400)", verticalAlign: "-2px",
          marginLeft: 1, animation: "lthn-cursor 1s steps(2) infinite",
        }} />
      )}
    </>
  );
}

function PanelFooter({ state }) {
  const cfg = {
    "first-run":  { tone: "idle",  pulse: false, label: "Idle · no model loaded" },
    "loading":    { tone: "warn",  pulse: false, label: "Loading model…" },
    "ready":      { tone: "ok",    pulse: false, label: "Model loaded · Airplane-mode OK" },
    "generating": { tone: "busy",  pulse: true,  label: "Generating" },
    "error":      { tone: "error", pulse: false, label: "Error: out of memory" },
  }[state];

  return (
    <div style={{
      height: 32, padding: "0 12px", flexShrink: 0,
      display: "flex", alignItems: "center", justifyContent: "space-between",
      borderTop: "1px solid var(--line-1)",
      background: "var(--ink-1)",
    }}>
      <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
        <StatusDot tone={cfg.tone} pulse={cfg.pulse} />
        <span style={{
          fontFamily: "var(--font-sans)", fontSize: 11,
          color: cfg.tone === "error" ? "var(--danger-400)" : "var(--fg-2)",
        }}>{cfg.label}</span>
      </div>
      <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
        <svg width="11" height="11" viewBox="0 0 11 11" aria-hidden="true">
          <path d="M5.5 0.8 L6.4 4.2 L10 5.5 L6.4 6.4 L5.5 9.8 L4.6 6.4 L1 5.5 L4.6 4.2 Z"
            fill="none" stroke="var(--success-400)" strokeWidth="0.9" strokeLinejoin="round" opacity="0.85" />
        </svg>
        <span style={{
          fontFamily: "var(--font-mono)", fontSize: 9.5,
          color: "var(--fg-3)", letterSpacing: "0.02em",
        }}>v0.1.0</span>
      </div>
    </div>
  );
}

/* ── Panel ────────────────────────────────────────────────────────── */

function TrayPanel({ state = "ready", showCursor = false }) {
  const stats = {
    "first-run":  { model: "—",                        tps: "—",     power: "—",     memory: "—" },
    "loading":    { model: "gemma-4-e2b-assistant",    tps: "—",     power: "1.2 W", memory: "0.8 / 2.1 GB" },
    "ready":      { model: "gemma-4-e2b-assistant",    tps: "—",     power: "1.4 W", memory: "2.1 GB" },
    "generating": { model: "gemma-4-e2b-assistant",    tps: "47.3",  power: "6.8 W", memory: "2.3 GB" },
    "error":      { model: "gemma-4-e2b-assistant",    tps: "—",     power: "1.4 W", memory: "2.1 GB" },
  }[state];

  const prompts = {
    "first-run":  "",
    "loading":    "",
    "ready":      "",
    "generating": "Summarise the architectural pattern that makes the tray panel a transient surface, not a navigation hub.",
    "error":      "Run the 14B model at full context.",
  };

  const output = {
    "generating": {
      live: true,
      tokens: 142,
      body: {
        before: "The tray panel is a ",
        selection: "single screen with no internal navigation",
        after: ". Closing all windows does not quit the app — the runner state persists in the tray process. Any window that opens later (settings, benchmark, model browser) is a transient surface ancho",
      },
    },
    "ready": {
      live: false,
      tokens: 142,
      body: {
        before: "The tray panel is a ",
        selection: "single screen with no internal navigation",
        after: ". Closing all windows does not quit the app — the runner state persists in the tray process.",
      },
    },
  }[state];

  return (
    <div data-platform="darwin" style={{
      width: 400, height: 560,
      background: "var(--ink-2)",
      border: "1px solid var(--line-2)",
      borderRadius: 11,
      overflow: "hidden",
      display: "flex", flexDirection: "column",
      boxShadow: "0 20px 60px rgba(0,0,0,0.55), 0 2px 0 rgba(255,255,255,0.04) inset",
      fontFamily: "var(--font-sans)",
      color: "var(--fg-0)",
      position: "relative",
    }}>
      <PanelHeader running={state === "generating"} canStart={state !== "first-run"} />
      <StatsStrip stats={stats} />
      <PromptArea state={state} prompt={prompts[state]} output={output} showCursor={showCursor} />
      <PanelFooter state={state} />
    </div>
  );
}

/* ── macOS menubar mockup ────────────────────────────────────────────
 * Shows the lthn icon nested among real menu-bar items, in both light
 * and dark menubar contexts, in both static and active variants.
 * ─────────────────────────────────────────────────────────────────── */
function MenubarStrip({ theme = "dark", active = false, label, showPopover = false, popoverState }) {
  const isDark = theme === "dark";
  // macOS uses a translucent dark/light material; we approximate with solids
  const bg = isDark
    ? "linear-gradient(to bottom, #28282e 0%, #1f1f24 100%)"
    : "linear-gradient(to bottom, #f3f3f5 0%, #e6e6ea 100%)";
  const fg = isDark ? "#e9e9ec" : "#1d1d1f";
  const fgMute = isDark ? "rgba(255,255,255,0.55)" : "rgba(0,0,0,0.55)";
  const iconColor = isDark ? "#e9e9ec" : "#1d1d1f";

  // The active dot uses brand violet; on a light bar we boost lightness so it still pops.
  const accent = "var(--brand-400)";

  return (
    <div style={{ width: "100%", display: "flex", flexDirection: "column", gap: 14 }}>
      {label && (
        <div style={{
          fontFamily: "var(--font-mono)", fontSize: 10.5,
          color: "var(--fg-3)", letterSpacing: "0.08em",
          textTransform: "uppercase",
        }}>{label}</div>
      )}
      <div style={{
        height: 26, padding: "0 10px",
        display: "flex", alignItems: "center", justifyContent: "space-between",
        background: bg,
        borderTop: isDark ? "1px solid rgba(255,255,255,0.06)" : "1px solid rgba(0,0,0,0.06)",
        borderBottom: isDark ? "1px solid rgba(0,0,0,0.4)" : "1px solid rgba(0,0,0,0.08)",
        fontFamily: '-apple-system, "SF Pro Text", system-ui, sans-serif',
        fontSize: 13,
        color: fg,
        position: "relative",
      }}>
        {/* Left: apple + app menu */}
        <div style={{ display: "flex", alignItems: "center", gap: 14 }}>
          <svg width="13" height="14" viewBox="0 0 14 16" fill={fg} aria-hidden="true">
            <path d="M9.3 2.5c.5-.6.8-1.5.7-2.4-.8.04-1.6.4-2.2 1-.5.5-.9 1.4-.8 2.3.8.07 1.7-.4 2.3-.9zm1.6 3c-1.2-.07-2.2.7-2.8.7-.6 0-1.5-.7-2.5-.6-1.3.02-2.5.8-3.1 2-1.4 2.3-.4 5.6 1 7.4.7.9 1.5 1.8 2.5 1.8 1 0 1.4-.6 2.6-.6 1.2 0 1.6.6 2.6.6 1.1 0 1.8-.9 2.5-1.8.8-1 1.1-2 1.1-2 0 0-2.1-.8-2.1-3.2 0-2 1.6-2.9 1.7-3-1-1.4-2.4-1.5-2.9-1.5z"/>
          </svg>
          <span style={{ fontWeight: 600 }}>lthn</span>
          <span style={{ color: fgMute }}>File</span>
          <span style={{ color: fgMute }}>Edit</span>
          <span style={{ color: fgMute }}>Window</span>
          <span style={{ color: fgMute }}>Help</span>
        </div>
        {/* Right: menulets — battery, wifi, clock, with our lthn icon inserted */}
        <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
          {/* lthn icon — the star of the show */}
          <div style={{
            position: "relative",
            padding: "3px 5px",
            borderRadius: 4,
            background: showPopover ? (isDark ? "rgba(255,255,255,0.12)" : "rgba(0,0,0,0.08)") : "transparent",
          }}>
            <LthnGlyph size={15} color={iconColor} active={active} accent={accent} />
          </div>
          {/* Battery */}
          <div style={{ display: "flex", alignItems: "center", gap: 3, opacity: 0.85 }}>
            <span style={{ fontSize: 11, color: fg }}>78%</span>
            <svg width="22" height="11" viewBox="0 0 22 11" fill="none">
              <rect x="0.5" y="0.5" width="18" height="10" rx="2" stroke={fg} strokeWidth="1" opacity="0.6" />
              <rect x="2" y="2" width="11" height="7" rx="1" fill={fg} />
              <rect x="19.5" y="3.5" width="1.5" height="4" rx="0.6" fill={fg} opacity="0.6" />
            </svg>
          </div>
          {/* Wifi */}
          <svg width="14" height="11" viewBox="0 0 14 11" fill={fg} aria-hidden="true">
            <path d="M7 1.5C4.4 1.5 2 2.5.4 4.1l1 1A7.5 7.5 0 0 1 7 3a7.5 7.5 0 0 1 5.6 2.1l1-1A9 9 0 0 0 7 1.5z" opacity="0.9"/>
            <path d="M7 4.5c-1.9 0-3.6.7-4.9 1.9l1 1A6 6 0 0 1 7 6c1.5 0 2.9.5 4 1.4l1-1A7.5 7.5 0 0 0 7 4.5z" opacity="0.9"/>
            <circle cx="7" cy="9" r="1.3" />
          </svg>
          {/* Clock */}
          <span style={{ fontSize: 12.5, color: fg, fontVariantNumeric: "tabular-nums" }}>Tue 12 May  14:32</span>
        </div>

        {/* Popover anchor preview — small triangle pointing up */}
        {showPopover && (
          <div style={{
            position: "absolute", top: "calc(100% - 1px)",
            right: 100, width: 12, height: 6,
            overflow: "hidden",
          }}>
            <div style={{
              width: 12, height: 12, background: "var(--ink-2)",
              border: "1px solid var(--line-2)",
              transform: "rotate(45deg) translateY(-6px)",
              transformOrigin: "center",
            }} />
          </div>
        )}
      </div>

      {/* The popover body itself, if shown */}
      {showPopover && (
        <div style={{ display: "flex", justifyContent: "flex-end", paddingRight: 60, marginTop: -8 }}>
          <TrayPanel state={popoverState || "ready"} />
        </div>
      )}
    </div>
  );
}

/* Inject keyframes once */
(function injectLthnKeyframes() {
  if (typeof document === "undefined") return;
  if (document.getElementById("lthn-panel-kf")) return;
  const s = document.createElement("style");
  s.id = "lthn-panel-kf";
  s.textContent = `
    @keyframes lthn-pulse {
      0%   { box-shadow: 0 0 0 0    color-mix(in oklab, var(--success-400) 55%, transparent); }
      70%  { box-shadow: 0 0 0 7px  color-mix(in oklab, var(--success-400)  0%, transparent); }
      100% { box-shadow: 0 0 0 0    color-mix(in oklab, var(--success-400)  0%, transparent); }
    }
    @keyframes lthn-cursor {
      0%, 50%  { opacity: 1; }
      51%, 100% { opacity: 0; }
    }
  `;
  document.head.appendChild(s);
})();

Object.assign(window, {
  LthnGlyph, LthnWordmark, StatusDot, Sparkline,
  TrayPanel, MenubarStrip,
});
