/* desktop-spec.jsx
 * Spec cards for the P0 deliverables — sit alongside the live mocks
 * on the canvas so engineering can read off measurements + animation
 * notes directly. Lethean-3 chrome, mono labels, dimensioned.
 */

/* Reusable: a captioned spec card with header band */
function SpecCard({ id, label, title, hint, children, dark = true, padding = 20 }) {
  return (
    <div style={{
      width: "100%", height: "100%",
      background: dark ? "var(--ink-2)" : "var(--ink-1)",
      color: "var(--fg-0)",
      fontFamily: "var(--font-sans)",
      display: "flex", flexDirection: "column",
    }}>
      <div style={{
        padding: "14px 20px",
        borderBottom: "1px solid var(--line-1)",
        display: "flex", justifyContent: "space-between", alignItems: "baseline",
      }}>
        <div>
          <div style={{
            fontFamily: "var(--font-mono)", fontSize: 10.5,
            color: "var(--fg-3)", letterSpacing: "0.08em",
            textTransform: "uppercase", marginBottom: 4,
          }}>{label}</div>
          <div style={{ fontSize: 17, fontWeight: 500, color: "var(--fg-0)", letterSpacing: "-0.01em" }}>{title}</div>
        </div>
        {hint && (
          <div style={{
            fontFamily: "var(--font-mono)", fontSize: 10.5,
            color: "var(--fg-3)", letterSpacing: "0.02em",
          }}>{hint}</div>
        )}
      </div>
      <div style={{ flex: 1, padding, minHeight: 0, overflow: "hidden" }}>
        {children}
      </div>
    </div>
  );
}

/* ── Icon spec ──────────────────────────────────────────────────────
 * The 4 SVG variants — light static, dark static, light active, dark
 * active — plus 16/32/64 size proofs, plus the construction grid.
 * ─────────────────────────────────────────────────────────────────── */
function IconSpecCard() {
  const swatches = [
    { theme: "Light bar",  bg: "linear-gradient(to bottom, #f3f3f5, #e6e6ea)", color: "#1d1d1f", label: "menu-bar-light.svg",  active: false },
    { theme: "Dark bar",   bg: "linear-gradient(to bottom, #28282e, #1f1f24)", color: "#e9e9ec", label: "menu-bar-dark.svg",   active: false },
    { theme: "Light · active", bg: "linear-gradient(to bottom, #f3f3f5, #e6e6ea)", color: "#1d1d1f", label: "menu-bar-light-active.svg", active: true },
    { theme: "Dark · active",  bg: "linear-gradient(to bottom, #28282e, #1f1f24)", color: "#e9e9ec", label: "menu-bar-dark-active.svg",  active: true },
  ];

  return (
    <SpecCard
      label="P0.1 · Tray icon"
      title="Lethean menu-bar glyph"
      hint="16×16 base · template-image compatible"
    >
      <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 16, height: "100%" }}>
        {/* Left: the four variants */}
        <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 10 }}>
          {swatches.map((s) => (
            <div key={s.label} style={{
              borderRadius: 8, border: "1px solid var(--line-1)",
              padding: 14, background: "var(--ink-1)",
              display: "flex", flexDirection: "column", gap: 10,
            }}>
              <div style={{
                background: s.bg, height: 28, borderRadius: 4,
                display: "flex", alignItems: "center", justifyContent: "center",
              }}>
                <LthnGlyph size={15} color={s.color} active={s.active} />
              </div>
              <div style={{
                background: s.bg, height: 64, borderRadius: 4,
                display: "flex", alignItems: "center", justifyContent: "center",
              }}>
                <LthnGlyph size={48} color={s.color} active={s.active} />
              </div>
              <div style={{
                fontFamily: "var(--font-mono)", fontSize: 9.5,
                color: "var(--fg-3)", letterSpacing: "0.02em",
              }}>{s.label}</div>
            </div>
          ))}
        </div>

        {/* Right: construction grid + spec */}
        <div style={{ display: "flex", flexDirection: "column", gap: 14 }}>
          <div style={{
            borderRadius: 8, border: "1px solid var(--line-1)",
            background: "var(--ink-1)", padding: 16,
            display: "flex", gap: 16, alignItems: "center",
          }}>
            {/* construction grid at 12× scale */}
            <div style={{
              width: 192, height: 192, position: "relative",
              background: "var(--ink-3)",
              borderRadius: 6, overflow: "hidden",
            }}>
              {/* grid */}
              <svg width="192" height="192" viewBox="0 0 16 16" style={{ position: "absolute", inset: 0 }}>
                {[...Array(15)].map((_, i) => (
                  <line key={`v${i}`} x1={i + 1} y1="0" x2={i + 1} y2="16"
                    stroke="var(--line-1)" strokeWidth="0.04" />
                ))}
                {[...Array(15)].map((_, i) => (
                  <line key={`h${i}`} x1="0" y1={i + 1} x2="16" y2={i + 1}
                    stroke="var(--line-1)" strokeWidth="0.04" />
                ))}
                {/* live area inset */}
                <rect x="2" y="2" width="12" height="12" fill="none"
                  stroke="var(--brand-500)" strokeWidth="0.08" strokeDasharray="0.4 0.4" opacity="0.5" />
              </svg>
              <div style={{ position: "absolute", inset: 0, display: "flex", alignItems: "center", justifyContent: "center" }}>
                <LthnGlyph size={192} color="var(--fg-0)" active={true} />
              </div>
            </div>
            <div style={{ fontSize: 11.5, lineHeight: 1.6, color: "var(--fg-2)" }}>
              <div style={{ color: "var(--fg-0)", fontWeight: 500, marginBottom: 6 }}>Construction</div>
              <div>Official brand mark · front-facing</div>
              <div>Aspect 1500 : 2123 · centred in 16×16 box</div>
              <div>Rendered as CSS-masked PNG · inherits colour</div>
              <div>Runner node = top-right · brand teal</div>
              <div style={{ marginTop: 8, color: "var(--fg-3)", fontFamily: "var(--font-mono)", fontSize: 10.5, letterSpacing: "0.02em" }}>
                hoplite_black.png · template-image mask<br />
                fill = #000 on light · #fff on dark<br />
                runner node = var(--brand-400) · #40c1c5
              </div>
            </div>
          </div>

          <div style={{
            borderRadius: 8, border: "1px solid var(--line-1)",
            background: "var(--ink-1)", padding: 16,
            fontSize: 11.5, lineHeight: 1.6, color: "var(--fg-2)",
          }}>
            <div style={{ color: "var(--fg-0)", fontWeight: 500, marginBottom: 6 }}>Rationale</div>
            The Lethean mark is a Corinthian hoplite helmet — from the brand
            guidelines, dropped in unmodified. Front profile with the T-slit
            already silhouetted; we render it as a CSS-masked image so it
            inherits the surrounding text colour and punches through any
            menu-bar background. The teal runner node only shows when the
            model is loaded — same metaphor as a power indicator. No brain,
            no sparkle, no robot.
          </div>
        </div>
      </div>
    </SpecCard>
  );
}

/* ── Sparkline spec ─────────────────────────────────────────────── */
function SparklineSpecCard() {
  return (
    <SpecCard
      label="P0.4 · Sparkline"
      title="Tok/s history · inline"
      hint="60 × 20 · ~12 samples · 1.6s pulse head"
    >
      <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 16, height: "100%" }}>
        {/* Anatomy at 6× */}
        <div style={{
          borderRadius: 8, border: "1px solid var(--line-1)",
          background: "var(--ink-1)", padding: 18,
          display: "flex", flexDirection: "column", gap: 12,
        }}>
          <div style={{
            fontFamily: "var(--font-mono)", fontSize: 10,
            color: "var(--fg-3)", letterSpacing: "0.08em",
            textTransform: "uppercase",
          }}>Anatomy · 6×</div>
          <div style={{
            background: "var(--ink-3)", borderRadius: 6,
            padding: "16px 16px 28px",
            position: "relative",
          }}>
            <svg width="360" height="120" viewBox="0 0 60 20" style={{ overflow: "visible" }}>
              {/* baseline grid */}
              <line x1="0" y1="18" x2="60" y2="18" stroke="var(--line-1)" strokeWidth="0.2" />
              <line x1="0" y1="10" x2="60" y2="10" stroke="var(--line-1)" strokeWidth="0.15" strokeDasharray="0.4 0.6" />
              <line x1="0" y1="2"  x2="60" y2="2"  stroke="var(--line-1)" strokeWidth="0.2" />
              {/* area + line */}
              <path d="M0 14 L5.45 11.4 L10.9 9 L16.36 6.5 L21.8 5 L27.27 4.1 L32.7 3.6 L38.18 2.9 L43.6 2.4 L49.09 2.1 L54.5 2.1 L60 2.1 L60 20 L0 20 Z" fill="var(--brand-400)" opacity="0.14" />
              <path d="M0 14 L5.45 11.4 L10.9 9 L16.36 6.5 L21.8 5 L27.27 4.1 L32.7 3.6 L38.18 2.9 L43.6 2.4 L49.09 2.1 L54.5 2.1 L60 2.1" fill="none" stroke="var(--brand-400)" strokeWidth="0.25" strokeLinecap="round" strokeLinejoin="round" />
              <circle cx="60" cy="2.1" r="0.5" fill="var(--brand-400)" />
              <circle cx="60" cy="2.1" r="0.5" fill="var(--brand-400)" opacity="0.4">
                <animate attributeName="r" values="0.5;1.6;0.5" dur="1.6s" repeatCount="indefinite" />
                <animate attributeName="opacity" values="0.5;0;0.5" dur="1.6s" repeatCount="indefinite" />
              </circle>

              {/* annotations */}
              <line x1="60" y1="2.1" x2="64" y2="2.1" stroke="var(--fg-3)" strokeWidth="0.1" />
              <line x1="60" y1="14"  x2="64" y2="14"  stroke="var(--fg-3)" strokeWidth="0.1" />
              <text x="65" y="2.6"  fontSize="1.8" fill="var(--fg-2)" fontFamily="var(--font-mono)">47 t/s</text>
              <text x="65" y="14.6" fontSize="1.8" fill="var(--fg-2)" fontFamily="var(--font-mono)">12 t/s</text>
            </svg>
          </div>

          <div style={{
            fontFamily: "var(--font-mono)", fontSize: 10,
            color: "var(--fg-3)", letterSpacing: "0.08em",
            textTransform: "uppercase",
          }}>Actual size · 1×</div>
          <div style={{
            background: "var(--ink-3)", borderRadius: 6,
            padding: "12px 14px",
            display: "flex", alignItems: "center", gap: 12,
          }}>
            <Sparkline width={60} height={20} />
            <span style={{ fontFamily: "var(--font-mono)", fontSize: 10.5, color: "var(--fg-2)" }}>60 × 20 px</span>
          </div>
        </div>

        {/* Spec table */}
        <div style={{
          borderRadius: 8, border: "1px solid var(--line-1)",
          background: "var(--ink-1)", padding: 18,
          display: "flex", flexDirection: "column", gap: 12,
          fontSize: 11.5, lineHeight: 1.55, color: "var(--fg-2)",
        }}>
          <div style={{ color: "var(--fg-0)", fontWeight: 500 }}>Spec</div>
          <SpecRow k="Dimensions"  v="60 × 20 px (no internal padding)" />
          <SpecRow k="Samples"     v="12 most recent tok/s readings · ring buffer" />
          <SpecRow k="Update"      v="On every token · throttled to 30 fps max" />
          <SpecRow k="Stroke"      v="var(--brand-400) · 1.25 px · round caps" />
          <SpecRow k="Area fill"   v="var(--brand-400) · opacity 0.14" />
          <SpecRow k="Head dot"    v="r 1.8 px · solid · pulses to 5.5 / 0 opacity over 1.6 s" />
          <SpecRow k="Range"       v="Auto-scaled to min/max of buffer · y-axis hidden" />
          <SpecRow k="Hidden when" v="state ≠ generating · last value frozen 1 s after stop" />

          <div style={{ color: "var(--fg-0)", fontWeight: 500, marginTop: 8 }}>Why not</div>
          <div style={{ color: "var(--fg-3)" }}>
            No glow on the curve (looks like a stock chart, not a system tool).
            No filled bars (loses the trend-not-magnitude story).
            No axis labels (the value is rendered in mono beside the sparkline).
          </div>
        </div>
      </div>
    </SpecCard>
  );
}

function SpecRow({ k, v }) {
  return (
    <div style={{ display: "grid", gridTemplateColumns: "100px 1fr", gap: 12, alignItems: "baseline" }}>
      <span style={{
        fontFamily: "var(--font-mono)", fontSize: 10,
        color: "var(--fg-3)", letterSpacing: "0.06em",
        textTransform: "uppercase",
      }}>{k}</span>
      <span style={{ color: "var(--fg-1)" }}>{v}</span>
    </div>
  );
}

/* ── Output area spec ──────────────────────────────────────────── */
function OutputSpecCard() {
  return (
    <SpecCard
      label="P0.5 · Output area"
      title="Streaming text surface"
      hint="max-height 200 px · mono · auto-scroll"
    >
      <div style={{ display: "grid", gridTemplateColumns: "1.1fr 1fr", gap: 16, height: "100%" }}>
        {/* The output surface at scale + states */}
        <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
          <div style={{
            background: "var(--ink-1)", border: "1px solid var(--line-1)",
            borderRadius: 8, padding: 16,
          }}>
            <div style={{
              fontFamily: "var(--font-mono)", fontSize: 10,
              color: "var(--fg-3)", letterSpacing: "0.08em",
              textTransform: "uppercase", marginBottom: 10,
            }}>Streaming · selection · copy hover</div>
            <div style={{
              background: "var(--ink-3)",
              border: "1px solid var(--line-1)",
              borderRadius: 6, overflow: "hidden",
              height: 200,
              display: "flex", flexDirection: "column",
            }}>
              <div style={{
                display: "flex", justifyContent: "space-between", alignItems: "center",
                padding: "6px 10px",
                borderBottom: "1px solid var(--line-1)",
                background: "var(--ink-1)",
              }}>
                <span style={{
                  fontFamily: "var(--font-mono)", fontSize: 9.5,
                  color: "var(--fg-3)", letterSpacing: "0.08em",
                  textTransform: "uppercase",
                }}>Output</span>
                <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
                  <Sparkline width={60} height={16} />
                  <span style={{ fontFamily: "var(--font-mono)", fontSize: 10, color: "var(--fg-2)" }}>47 t/s</span>
                  <button style={{
                    appearance: "none", background: "transparent",
                    border: "1px solid var(--line-2)", color: "var(--fg-2)",
                    height: 18, padding: "0 6px", borderRadius: 3,
                    fontFamily: "var(--font-mono)", fontSize: 9.5,
                    letterSpacing: "0.04em", cursor: "pointer",
                  }}>COPY</button>
                </div>
              </div>
              <div style={{
                flex: 1, padding: "10px 12px", overflow: "auto",
                fontFamily: "var(--font-mono)", fontSize: 12, lineHeight: 1.55,
                color: "var(--fg-1)",
              }}>
                The tray panel is a{" "}
                <span style={{
                  background: "color-mix(in oklab, var(--brand-500) 35%, transparent)",
                  color: "var(--fg-0)",
                }}>single screen with no internal navigation</span>
                . Closing all windows does not quit the app — the runner state persists in the tray process. Any window that opens later (settings, benchmark, model browser) is a transient surface anchored to the tray-process lifetime, never a sub-view inside the popover
                <span style={{
                  display: "inline-block", width: 6, height: 13,
                  background: "var(--brand-400)", verticalAlign: "-2px",
                  marginLeft: 1, animation: "lthn-cursor 1s steps(2) infinite",
                }} />
              </div>
            </div>
          </div>

          <div style={{
            background: "var(--ink-1)", border: "1px solid var(--line-1)",
            borderRadius: 8, padding: 16,
          }}>
            <div style={{
              fontFamily: "var(--font-mono)", fontSize: 10,
              color: "var(--fg-3)", letterSpacing: "0.08em",
              textTransform: "uppercase", marginBottom: 10,
            }}>Code block · v0 plain mono</div>
            <div style={{
              background: "var(--ink-3)",
              border: "1px solid var(--line-1)",
              borderRadius: 6, padding: "10px 12px",
              fontFamily: "var(--font-mono)", fontSize: 12, lineHeight: 1.55,
              color: "var(--fg-1)", position: "relative",
            }}>
              <div style={{ color: "var(--fg-3)" }}>{`# Pull the starter model`}</div>
              <div>lthn ai models pull gemma-4-e2b-assistant</div>
              <div style={{ color: "var(--fg-3)" }}>{`# Then run it`}</div>
              <div>lthn ai serve</div>
              <button style={{
                position: "absolute", top: 8, right: 8,
                appearance: "none", background: "var(--ink-1)",
                border: "1px solid var(--line-2)", color: "var(--fg-2)",
                height: 20, padding: "0 7px", borderRadius: 4,
                fontFamily: "var(--font-mono)", fontSize: 9.5,
                letterSpacing: "0.04em", cursor: "pointer",
              }}>COPY</button>
            </div>
          </div>
        </div>

        {/* Spec column */}
        <div style={{
          borderRadius: 8, border: "1px solid var(--line-1)",
          background: "var(--ink-1)", padding: 18,
          display: "flex", flexDirection: "column", gap: 10,
          fontSize: 11.5, lineHeight: 1.55, color: "var(--fg-2)",
        }}>
          <div style={{ color: "var(--fg-0)", fontWeight: 500 }}>Spec</div>
          <SpecRow k="Font"        v="var(--font-mono) · 12 / 1.55" />
          <SpecRow k="Max height"  v="200 px · scroll within · panel does not grow" />
          <SpecRow k="Auto-scroll" v="Pin to bottom while user not scrolling · release on wheel up" />
          <SpecRow k="Selection"   v="color-mix(brand-500 35%, transparent) · fg-0" />
          <SpecRow k="Live cursor" v="6 × 13 px brand-400 · steps(2) 1 s blink · while generating only" />
          <SpecRow k="Copy"        v="Always-visible button top-right · code block also shows inline COPY" />
          <SpecRow k="Code blocks" v="v0 plain mono · comment lines fg-3 · highlighter is v0.3" />
          <SpecRow k="Scroll bar"  v="Native macOS overlay · no custom styling" />

          <div style={{ color: "var(--fg-0)", fontWeight: 500, marginTop: 6 }}>Reset rules</div>
          <SpecRow k="Clear"       v="On Generate · on model switch · on Stop after 60 s" />
          <SpecRow k="Persist"     v="Last response kept across popover hide/show until cleared" />
        </div>
      </div>
    </SpecCard>
  );
}

/* ── State-grid caption strip ─────────────────────────────────── */
function StateCaption({ name, blurb }) {
  return (
    <div style={{
      width: 400, marginTop: 14,
      fontFamily: "var(--font-sans)",
    }}>
      <div style={{
        fontFamily: "var(--font-mono)", fontSize: 10.5,
        color: "var(--brand-300)", letterSpacing: "0.08em",
        textTransform: "uppercase", marginBottom: 6,
      }}>{name}</div>
      <div style={{ fontSize: 12, color: "var(--fg-2)", lineHeight: 1.45 }}>{blurb}</div>
    </div>
  );
}

/* A single state mock: the panel + caption underneath */
function StateMock({ state, name, blurb, showCursor }) {
  return (
    <div style={{ display: "flex", flexDirection: "column", alignItems: "center" }}>
      <TrayPanel state={state} showCursor={showCursor} />
      <StateCaption name={name} blurb={blurb} />
    </div>
  );
}

/* ── Open-questions card ─────────────────────────────────────────
 * The 8 questions for Snider, with my proposed call beside each.
 * ─────────────────────────────────────────────────────────────── */
function OpenQuestionsCard() {
  const items = [
    { q: "Tray icon shape",       a: "Official brand hoplite helmet (hoplite_black.png from the guidelines) — rendered as a CSS-masked image so it inherits text colour and stays template-image-safe on any menu bar. Active state adds the teal runner node." },
    { q: "Sparkline aesthetic",   a: "Line + 14% area fill, no glow on the curve. Only the trailing head dot pulses (1.6 s). Steady-state when not generating." },
    { q: "Error tone",            a: "Static red dot for colour-blind safety, but a triangle exclamation glyph replaces the dot shape — encodes severity in shape AND colour." },
    { q: "Code blocks",           a: "Plain mono for v0. Comment lines in fg-3. Inline COPY button. Syntax highlighting is v0.3." },
    { q: "Welcome model pick",    a: "Curated 3-up: Gemma 4 E2B (recommended), Llama 3.2 3B, Phi 3.5 Mini. Auto-select Gemma. Skip-link goes to model browser." },
    { q: "Settings density",      a: "One window, sectioned (omlx pattern, Lethean-3 chrome). Per-model settings live as a sub-card, not a separate window." },
    { q: "API key",               a: "Generated at first run (lthn-sk-…), shown once with copy-confirm. Regenerable in Settings · API. Stored in macOS Keychain." },
    { q: "Vi avatar in panel",    a: "Presence only — no avatar inside the 400×560 (too cramped). Vi appears in the Welcome window header and in error-state copy ('I couldn't load that')." },
  ];

  return (
    <SpecCard
      label="§6 · Open questions"
      title="Decisions to confirm with Snider"
      hint="my call beside each · flag any to revisit"
    >
      <div style={{
        display: "grid", gridTemplateColumns: "1fr 1fr", gap: 14,
        height: "100%", overflow: "hidden",
      }}>
        {items.map((it, i) => (
          <div key={i} style={{
            borderRadius: 8, border: "1px solid var(--line-1)",
            background: "var(--ink-1)", padding: 14,
            display: "flex", flexDirection: "column", gap: 6,
          }}>
            <div style={{ display: "flex", gap: 8, alignItems: "baseline" }}>
              <span style={{
                fontFamily: "var(--font-mono)", fontSize: 10,
                color: "var(--brand-300)", letterSpacing: "0.04em",
              }}>Q{i + 1}</span>
              <span style={{ fontSize: 12.5, fontWeight: 500, color: "var(--fg-0)" }}>{it.q}</span>
            </div>
            <div style={{ fontSize: 11.5, lineHeight: 1.5, color: "var(--fg-2)" }}>{it.a}</div>
          </div>
        ))}
      </div>
    </SpecCard>
  );
}

/* ── Brief card — what's in scope this week ───────────────────── */
function BriefIntroCard() {
  return (
    <div style={{
      width: "100%", height: "100%",
      background: "var(--ink-2)",
      color: "var(--fg-0)",
      fontFamily: "var(--font-sans)",
      padding: "36px 44px",
      display: "flex", flexDirection: "column", gap: 24,
    }}>
      <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
        <LthnGlyph size={26} color="var(--fg-0)" active={true} />
        <div style={{
          fontFamily: "var(--font-mono)", fontSize: 11.5,
          color: "var(--fg-3)", letterSpacing: "0.1em",
          textTransform: "uppercase",
        }}>Lethean Desktop · Week-of design lane</div>
      </div>

      <div style={{ display: "grid", gridTemplateColumns: "1.3fr 1fr", gap: 40 }}>
        <div>
          <div style={{
            fontFamily: "var(--font-serif, var(--font-sans))",
            fontStyle: "italic",
            fontSize: 32, lineHeight: 1.2, color: "var(--fg-0)",
            letterSpacing: "-0.015em",
            textWrap: "pretty",
          }}>
            Sovereign compute, calm presence. <br />
            A menu-bar runner, four numbers, one prompt — and nothing else trying to be a hub.
          </div>
          <div style={{ marginTop: 22, color: "var(--fg-2)", fontSize: 13.5, lineHeight: 1.6, maxWidth: 560 }}>
            P0 covers the first-release demo surface: the tray icon, the 400×560 popover,
            and the five meaningful states the runner moves through. Everything else (welcome,
            settings, model browser, web admin) opens as a transient window from this panel —
            so this is the only screen we cannot get wrong.
          </div>
        </div>

        <div style={{
          background: "var(--ink-1)", border: "1px solid var(--line-1)",
          borderRadius: 10, padding: 18,
          display: "flex", flexDirection: "column", gap: 10,
          fontSize: 12, lineHeight: 1.5, color: "var(--fg-2)",
        }}>
          <div style={{
            fontFamily: "var(--font-mono)", fontSize: 10.5,
            color: "var(--fg-3)", letterSpacing: "0.08em",
            textTransform: "uppercase",
          }}>This week · P0</div>
          {[
            ["P0.1", "Tray icon SVG family (4 variants)"],
            ["P0.2", "400 × 560 popover panel · 5 sections"],
            ["P0.3", "5 state variants (first-run · loading · ready · generating · error)"],
            ["P0.4", "Sparkline anatomy + animation spec"],
            ["P0.5", "Output area · selection · copy · code blocks"],
          ].map(([k, v]) => (
            <div key={k} style={{ display: "flex", gap: 10 }}>
              <span style={{
                fontFamily: "var(--font-mono)", fontSize: 10,
                color: "var(--brand-300)", letterSpacing: "0.04em",
                width: 32, flexShrink: 0,
              }}>{k}</span>
              <span style={{ color: "var(--fg-1)" }}>{v}</span>
            </div>
          ))}
          <div style={{
            marginTop: 8, paddingTop: 12,
            borderTop: "1px solid var(--line-1)",
            fontSize: 11, color: "var(--fg-3)", lineHeight: 1.5,
          }}>
            Lethean-3 dark · brand violet (hue 270) · SF Pro on Darwin · British English · Vi voice (calm, observational, never exclamatory).
          </div>
        </div>
      </div>
    </div>
  );
}

Object.assign(window, {
  SpecCard, IconSpecCard, SparklineSpecCard, OutputSpecCard,
  StateMock, StateCaption, OpenQuestionsCard, BriefIntroCard,
});
