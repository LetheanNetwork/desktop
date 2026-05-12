/* windows-ops.jsx — E1 operational windows
 * Welcome (720×560 min, 3 steps) · Settings (720×560 sectioned scroll) · Model Browser (1000×680 list+detail)
 */

/* ─────────────────────────────────────────────────────────────────
 * E1.1 · Welcome window — 3 steps
 * ───────────────────────────────────────────────────────────────── */
function WelcomeWindow({ step = 1, w = 760, h = 580 }) {
  return (
    <LthnWindow title="Welcome to lthn" subtitle={`step ${step} of 3`} width={w} height={h}
      footer={<>British English · dark default · accessibility light in Settings · v0.2.0-rc1</>}>
      <div style={{ flex: 1, display: "grid", gridTemplateColumns: "240px 1fr", minHeight: 0 }}>
        {/* Steps rail */}
        <aside style={{
          background: "rgba(0,0,0,0.18)",
          borderRight: "1px solid rgba(255,255,255,0.05)",
          padding: "26px 22px",
          display: "flex", flexDirection: "column", gap: 18,
        }}>
          <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
            <LthnGlyph size={24} color="var(--fg-0)" active={true} />
            <div>
              <div style={{ fontSize: 13, fontWeight: 600, color: "var(--fg-0)" }}>lthn</div>
              <div style={{ fontFamily: "var(--font-mono)", fontSize: 10, color: "var(--fg-3)", letterSpacing: "0.04em" }}>
                sovereign · single-watt
              </div>
            </div>
          </div>
          <div style={{ height: 1, background: "rgba(255,255,255,0.06)", margin: "4px 0" }} />
          {[
            { n: 1, label: "Model directory", hint: "Where models live" },
            { n: 2, label: "First model",     hint: "Pick a starter" },
            { n: 3, label: "Connect",         hint: "Wire up clients" },
          ].map((s) => {
            const done = s.n < step, here = s.n === step;
            return (
              <div key={s.n} style={{ display: "flex", gap: 12, alignItems: "flex-start" }}>
                <div style={{
                  width: 22, height: 22, borderRadius: "50%",
                  background: done ? "var(--brand-500)" : here ? "transparent" : "transparent",
                  border: here ? "1.5px solid var(--brand-400)" : done ? "1.5px solid var(--brand-500)" : "1.5px solid rgba(255,255,255,0.12)",
                  display: "flex", alignItems: "center", justifyContent: "center",
                  fontSize: 11, fontWeight: 600,
                  color: done ? "#fff" : here ? "var(--brand-300)" : "var(--fg-3)",
                  flexShrink: 0,
                }}>{done ? <i className="fa-solid fa-check" style={{ fontSize: 9 }} /> : s.n}</div>
                <div>
                  <div style={{ fontSize: 12, fontWeight: 500, color: here ? "var(--fg-0)" : "var(--fg-2)" }}>{s.label}</div>
                  <div style={{ fontSize: 10.5, color: "var(--fg-3)", marginTop: 2 }}>{s.hint}</div>
                </div>
              </div>
            );
          })}
          <div style={{ flex: 1 }} />
          <div style={{ fontSize: 10.5, color: "var(--fg-3)", lineHeight: 1.5 }}>
            You can change all of this later in Settings. Nothing leaves this Mac.
          </div>
        </aside>

        {/* Step body */}
        <main style={{ padding: "32px 40px", display: "flex", flexDirection: "column", minHeight: 0 }}>
          {step === 1 && <WelcomeStep1 />}
          {step === 2 && <WelcomeStep2 />}
          {step === 3 && <WelcomeStep3 />}
          <div style={{ flex: 1 }} />
          <div style={{ display: "flex", alignItems: "center", gap: 10, paddingTop: 18 }}>
            {step > 1 && <WinBtn tone="ghost" size="lg">Back</WinBtn>}
            <WinBtn tone="quiet" size="lg">Skip for now</WinBtn>
            <div style={{ flex: 1 }} />
            <WinBtn tone="primary" size="lg"
              icon={step === 3 ? <i className="fa-solid fa-check" /> : <i className="fa-solid fa-arrow-right" />}>
              {step === 3 ? "Finish" : step === 1 ? "Use this folder" : "Download & continue"}
            </WinBtn>
          </div>
        </main>
      </div>
    </LthnWindow>
  );
}

function WelcomeStep1() {
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 18 }}>
      <div>
        <div style={{ fontSize: 24, fontWeight: 600, color: "var(--fg-0)", letterSpacing: "-0.018em" }}>
          Where shall we keep your models?
        </div>
        <div style={{ fontSize: 13, color: "var(--fg-2)", marginTop: 8, lineHeight: 1.55, maxWidth: 440 }}>
          A folder on this Mac. Models can be big — pick somewhere with room.
          We default to your home directory; change it if you have a faster volume.
        </div>
      </div>
      <div style={{
        marginTop: 4,
        padding: "20px 22px",
        border: "1.5px dashed rgba(64,193,197,0.30)",
        borderRadius: 10,
        background: "rgba(64,193,197,0.04)",
        display: "flex", alignItems: "center", gap: 18,
      }}>
        <div style={{
          width: 44, height: 44, borderRadius: 10,
          background: "rgba(64,193,197,0.10)",
          border: "1px solid rgba(64,193,197,0.20)",
          display: "flex", alignItems: "center", justifyContent: "center",
        }}>
          <i className="fa-solid fa-folder-open" style={{ fontSize: 18, color: "var(--brand-300)" }} />
        </div>
        <div style={{ flex: 1 }}>
          <div style={{ fontFamily: "var(--font-mono)", fontSize: 13, color: "var(--fg-0)", letterSpacing: "-0.005em" }}>
            ~/.lthn/models/
          </div>
          <div style={{ fontSize: 11, color: "var(--fg-3)", marginTop: 2 }}>
            312 GB free on this volume · default location
          </div>
        </div>
        <WinBtn tone="ghost" size="md">Choose folder…</WinBtn>
      </div>
    </div>
  );
}

function WelcomeStep2() {
  const models = [
    { name: "Gemma 4 E2B (-assistant)", author: "Google", size: "2.1 GB", ram: "4 GB", desc: "Best balance for first run · Lethean-recommended", rec: true },
    { name: "Llama 3.2 3B Instruct",    author: "Meta",   size: "3.4 GB", ram: "6 GB", desc: "Solid general-purpose · longer context window" },
    { name: "Phi 3.5 Mini Instruct",    author: "Microsoft", size: "2.6 GB", ram: "5 GB", desc: "Punches above its weight on reasoning" },
  ];
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 16, minHeight: 0 }}>
      <div>
        <div style={{ fontSize: 24, fontWeight: 600, color: "var(--fg-0)", letterSpacing: "-0.018em" }}>
          Pick a model to start with.
        </div>
        <div style={{ fontSize: 13, color: "var(--fg-2)", marginTop: 8, lineHeight: 1.55, maxWidth: 460 }}>
          Three small models that run comfortably on Apple Silicon.
          You can add more from the model browser anytime.
        </div>
      </div>
      <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
        {models.map((m, i) => (
          <div key={m.name} style={{
            display: "flex", alignItems: "center", gap: 14,
            padding: "14px 16px",
            borderRadius: 10,
            background: m.rec ? "rgba(64,193,197,0.06)" : "rgba(255,255,255,0.03)",
            border: m.rec ? "1px solid rgba(64,193,197,0.22)" : "1px solid rgba(255,255,255,0.06)",
          }}>
            <div style={{
              width: 18, height: 18, borderRadius: "50%",
              border: "1.5px solid " + (m.rec ? "var(--brand-400)" : "rgba(255,255,255,0.18)"),
              display: "flex", alignItems: "center", justifyContent: "center",
              flexShrink: 0,
            }}>
              {m.rec && <div style={{ width: 8, height: 8, borderRadius: "50%", background: "var(--brand-400)" }} />}
            </div>
            <div style={{ flex: 1 }}>
              <div style={{ display: "flex", alignItems: "baseline", gap: 8 }}>
                <span style={{ fontSize: 13.5, fontWeight: 500, color: "var(--fg-0)", letterSpacing: "-0.005em" }}>{m.name}</span>
                <span style={{ fontSize: 11, color: "var(--fg-3)" }}>· {m.author}</span>
                {m.rec && <span style={{
                  fontFamily: "var(--font-mono)", fontSize: 9.5,
                  color: "var(--brand-300)", letterSpacing: "0.06em", textTransform: "uppercase",
                  padding: "2px 6px", borderRadius: 999,
                  background: "rgba(64,193,197,0.10)", border: "1px solid rgba(64,193,197,0.22)",
                }}>Recommended</span>}
              </div>
              <div style={{ fontSize: 11.5, color: "var(--fg-2)", marginTop: 3 }}>{m.desc}</div>
            </div>
            <div style={{ textAlign: "right", fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--fg-3)", letterSpacing: "0.02em" }}>
              <div>{m.size}</div>
              <div style={{ marginTop: 2 }}>{m.ram} RAM</div>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

function WelcomeStep3() {
  const clients = [
    { name: "Claude Code",  desc: "Anthropic's CLI · drop-in OpenAI-compatible endpoint", path: "~/.config/claude/config.json" },
    { name: "OpenCode",     desc: "Open-source coding agent",                              path: "~/.config/opencode/config.toml" },
    { name: "Codex",        desc: "OpenAI Codex CLI",                                      path: "~/.codex/config.yaml" },
  ];
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
      <div>
        <div style={{ fontSize: 24, fontWeight: 600, color: "var(--fg-0)", letterSpacing: "-0.018em" }}>
          Want to wire it into your tools?
        </div>
        <div style={{ fontSize: 13, color: "var(--fg-2)", marginTop: 8, lineHeight: 1.55, maxWidth: 460 }}>
          lthn speaks the OpenAI-compatible API on <span style={{ fontFamily: "var(--font-mono)", color: "var(--fg-1)" }}>http://localhost:8000/v1</span>.
          We can drop the endpoint into these configs for you. The only outbound action lthn ever takes without you asking.
        </div>
      </div>
      <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
        {clients.map((c) => (
          <div key={c.name} style={{
            display: "flex", alignItems: "center", gap: 14,
            padding: "12px 14px",
            borderRadius: 8,
            background: "rgba(255,255,255,0.03)",
            border: "1px solid rgba(255,255,255,0.06)",
          }}>
            <input type="checkbox" defaultChecked={c.name === "Claude Code"} style={{ accentColor: "var(--brand-400)" }} />
            <div style={{ flex: 1 }}>
              <div style={{ fontSize: 12.5, fontWeight: 500, color: "var(--fg-0)" }}>{c.name}</div>
              <div style={{ fontSize: 11, color: "var(--fg-3)", marginTop: 1, fontFamily: "var(--font-mono)", letterSpacing: "0.01em" }}>{c.path}</div>
            </div>
            <div style={{ fontSize: 11, color: "var(--fg-3)" }}>{c.desc}</div>
          </div>
        ))}
      </div>
    </div>
  );
}

/* ─────────────────────────────────────────────────────────────────
 * E1.2 · Settings window — sectioned scroll
 * ───────────────────────────────────────────────────────────────── */
function SettingsWindow({ open = "models", w = 760, h = 600 }) {
  const sections = [
    { id: "general",      icon: "fa-sliders",          label: "General" },
    { id: "models",       icon: "fa-cube",             label: "Models" },
    { id: "runner",       icon: "fa-gauge-high",       label: "Runner" },
    { id: "api",          icon: "fa-plug",             label: "API" },
    { id: "telemetry",    icon: "fa-wave-square",      label: "Telemetry" },
    { id: "integrations", icon: "fa-link",             label: "Integrations" },
    { id: "about",        icon: "fa-circle-info",      label: "About" },
  ];
  return (
    <LthnWindow title="Settings" subtitle="lthn · v0.2.0-rc1" width={w} height={h}
      footer={<>Changes apply immediately · ⌘W to close · the runner keeps running</>}>
      <div style={{ flex: 1, display: "grid", gridTemplateColumns: "200px 1fr", minHeight: 0 }}>
        {/* Section rail */}
        <aside style={{
          background: "rgba(0,0,0,0.18)",
          borderRight: "1px solid rgba(255,255,255,0.05)",
          padding: "12px 8px",
          display: "flex", flexDirection: "column", gap: 1,
        }}>
          {sections.map((s) => (
            <div key={s.id} style={{
              padding: "8px 12px",
              borderRadius: 6,
              background: s.id === open ? "rgba(255,255,255,0.07)" : "transparent",
              display: "flex", alignItems: "center", gap: 10,
              fontSize: 12.5, color: s.id === open ? "var(--fg-0)" : "var(--fg-2)",
              cursor: "pointer",
            }}>
              <i className={`fa-solid ${s.icon}`} style={{
                fontSize: 11, width: 14, textAlign: "center",
                color: s.id === open ? "var(--brand-300)" : "var(--fg-3)",
              }} />
              {s.label}
            </div>
          ))}
        </aside>

        {/* Body */}
        <main style={{ padding: "28px 32px", overflow: "auto", display: "flex", flexDirection: "column", gap: 22 }}>
          <SettingsSection
            title="Models" open
            desc="Where lthn looks for models and which one loads at startup."
          >
            <SettingsRow label="Model directory"
              control={
                <div style={{
                  display: "flex", alignItems: "center", gap: 8,
                  padding: "6px 10px", borderRadius: 6,
                  background: "rgba(255,255,255,0.04)",
                  border: "1px solid rgba(255,255,255,0.07)",
                  fontFamily: "var(--font-mono)", fontSize: 11.5, color: "var(--fg-1)",
                }}>
                  <i className="fa-regular fa-folder" style={{ fontSize: 11, color: "var(--fg-3)" }} />
                  ~/.lthn/models/
                  <WinBtn tone="quiet" size="sm" style={{ marginLeft: 4 }}>Change…</WinBtn>
                </div>
              }
            />
            <SettingsRow label="Default model"
              hint="Auto-loads when the runner starts. Empty = no auto-load."
              control={<SettingsSelect value="Gemma 4 E2B" />}
            />
            <SettingsRow label="Quantisation preference"
              hint="Pick the smallest quant your hardware comfortably runs."
              control={<SettingsSegment value="q4_k_m" options={["q4_0", "q4_k_m", "q5_k_m", "q8_0"]} />}
            />
            <SettingsRow label="Default sampling"
              hint="Per-model overrides live in the model browser."
              control={
                <div style={{ display: "flex", gap: 18, fontSize: 11.5, color: "var(--fg-2)" }}>
                  <span>Temp <span style={{ color: "var(--fg-0)", fontFamily: "var(--font-mono)" }}>0.7</span></span>
                  <span>Top-p <span style={{ color: "var(--fg-0)", fontFamily: "var(--font-mono)" }}>0.95</span></span>
                  <span>Max tok <span style={{ color: "var(--fg-0)", fontFamily: "var(--font-mono)" }}>1024</span></span>
                </div>
              }
            />
          </SettingsSection>

          <SettingsSection
            title="Runner"
            desc="How the inference process behaves. Don't change these unless you're sure."
          >
            <SettingsRow label="Max concurrent requests" control={<SettingsSelect value="4" />} />
            <SettingsRow label="Max process memory" hint="Soft cap — runner will refuse to load a model that pushes past this." control={<SettingsSelect value="24 GB" />} />
            <SettingsRow label="Context scaling" hint="Stretch context window beyond model default · uses YaRN." control={<SettingsToggle value={false} />} />
          </SettingsSection>

          <SettingsSection
            title="API"
            desc="HTTP server for OpenAI-compatible clients. Off by default; nothing leaves this Mac unless you turn it on."
          >
            <SettingsRow label="HTTP server" control={<SettingsToggle value={true} />} />
            <SettingsRow label="Endpoint"
              control={<span style={{ fontFamily: "var(--font-mono)", fontSize: 11.5, color: "var(--fg-1)" }}>http://localhost:8000/v1</span>}
            />
            <SettingsRow label="API key" hint="Required for any client connecting to the local server."
              control={
                <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
                  <span style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--fg-1)" }}>sk-lthn-••••••••••••••••2qB7</span>
                  <WinBtn tone="quiet" size="sm" icon={<i className="fa-regular fa-copy" style={{ fontSize: 10 }} />} />
                  <WinBtn tone="quiet" size="sm" icon={<i className="fa-solid fa-rotate-right" style={{ fontSize: 10 }} />} />
                </div>
              }
            />
          </SettingsSection>
        </main>
      </div>
    </LthnWindow>
  );
}

function SettingsSection({ title, desc, open = false, children }) {
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 14 }}>
      <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
        <i className={`fa-solid ${open ? "fa-angle-down" : "fa-angle-right"}`} style={{ fontSize: 11, color: "var(--fg-3)" }} />
        <div style={{ fontSize: 14.5, fontWeight: 600, color: "var(--fg-0)", letterSpacing: "-0.015em" }}>{title}</div>
      </div>
      {desc && <div style={{ fontSize: 11.5, color: "var(--fg-3)", lineHeight: 1.55, marginLeft: 20 }}>{desc}</div>}
      {open && (
        <div style={{
          marginLeft: 20,
          display: "flex", flexDirection: "column", gap: 14,
          padding: "8px 0",
          borderTop: "1px solid rgba(255,255,255,0.05)",
        }}>{children}</div>
      )}
    </div>
  );
}

function SettingsRow({ label, hint, control }) {
  return (
    <div style={{ display: "grid", gridTemplateColumns: "200px 1fr", gap: 18, alignItems: "flex-start", paddingTop: 8 }}>
      <div style={{ display: "flex", flexDirection: "column", gap: 3 }}>
        <div style={{ fontSize: 12.5, color: "var(--fg-1)", fontWeight: 500 }}>{label}</div>
        {hint && <div style={{ fontSize: 10.5, color: "var(--fg-3)", lineHeight: 1.5 }}>{hint}</div>}
      </div>
      <div>{control}</div>
    </div>
  );
}

function SettingsSelect({ value }) {
  return (
    <div style={{
      display: "inline-flex", alignItems: "center", gap: 8,
      padding: "6px 10px", borderRadius: 6,
      background: "rgba(255,255,255,0.04)",
      border: "1px solid rgba(255,255,255,0.07)",
      fontSize: 11.5, color: "var(--fg-1)",
    }}>
      {value}
      <i className="fa-solid fa-angle-down" style={{ fontSize: 9, color: "var(--fg-3)" }} />
    </div>
  );
}

function SettingsSegment({ value, options }) {
  return (
    <div style={{
      display: "inline-flex", borderRadius: 6,
      background: "rgba(0,0,0,0.18)",
      border: "1px solid rgba(255,255,255,0.06)",
      padding: 2,
    }}>
      {options.map((o) => (
        <div key={o} style={{
          padding: "4px 10px",
          fontFamily: "var(--font-mono)", fontSize: 10.5,
          color: o === value ? "var(--fg-0)" : "var(--fg-3)",
          background: o === value ? "rgba(255,255,255,0.08)" : "transparent",
          borderRadius: 4,
          letterSpacing: "0.02em",
        }}>{o}</div>
      ))}
    </div>
  );
}

function SettingsToggle({ value }) {
  return (
    <div style={{
      width: 32, height: 18, borderRadius: 999,
      background: value ? "var(--brand-500)" : "rgba(255,255,255,0.10)",
      position: "relative", transition: "background 0.15s",
    }}>
      <div style={{
        position: "absolute", top: 2, left: value ? 16 : 2,
        width: 14, height: 14, borderRadius: "50%", background: "#fff",
        boxShadow: "0 1px 2px rgba(0,0,0,0.3)",
      }} />
    </div>
  );
}

/* ─────────────────────────────────────────────────────────────────
 * E1.3 · Model browser window — list + search + detail
 * ───────────────────────────────────────────────────────────────── */
function ModelBrowserWindow({ w = 1040, h = 700, selected = "gemma-4-e2b" }) {
  const local = [
    { id: "gemma-4-e2b",    name: "gemma-4-e2b",    family: "Gemma",  size: "2.1 GB", status: "loaded" },
    { id: "llama-3.2-3b",   name: "llama-3.2-3b",   family: "Llama",  size: "3.4 GB", status: "available" },
    { id: "phi-3.5-mini",   name: "phi-3.5-mini",   family: "Phi",    size: "2.6 GB", status: "available" },
    { id: "qwen-2.5-7b",    name: "qwen-2.5-7b",    family: "Qwen",   size: "4.8 GB", status: "downloading" },
  ];
  const results = [
    { name: "Qwen2.5-Coder-7B-Instruct",    author: "Qwen",       size: "4.8 GB", q: "q4_k_m", family: "Coder", tools: true,  vision: false, downloads: "1.2M" },
    { name: "Mistral-Nemo-12B-Instruct",    author: "MistralAI",  size: "8.4 GB", q: "q4_k_m", family: "Mistral", tools: true,  vision: false, downloads: "420k" },
    { name: "Llama-3.2-11B-Vision-Instruct",author: "Meta",       size: "9.1 GB", q: "q4_k_m", family: "Llama",   tools: false, vision: true,  downloads: "880k" },
    { name: "Gemma-3-27B-IT",               author: "Google",     size: "16 GB",  q: "q4_k_m", family: "Gemma",   tools: false, vision: false, downloads: "340k" },
    { name: "Phi-4-14B-Instruct",           author: "Microsoft",  size: "9.6 GB", q: "q4_k_m", family: "Phi",     tools: true,  vision: false, downloads: "260k" },
  ];
  return (
    <LthnWindow title="Models" subtitle="local · 4 · huggingface" width={w} height={h}
      toolbar={<>
        <WinBtn tone="ghost" size="sm" icon={<i className="fa-solid fa-filter" style={{ fontSize: 10 }} />}>Filters</WinBtn>
        <WinBtn tone="primary" size="sm" icon={<i className="fa-solid fa-arrow-down" style={{ fontSize: 10 }} />}>Import GGUF…</WinBtn>
      </>}
      footer={<>4 local · 312 GB free · ~/.lthn/models/ · airplane-mode OK (browsing requires network)</>}
    >
      <div style={{ flex: 1, display: "grid", gridTemplateColumns: "240px 1fr 300px", minHeight: 0 }}>
        {/* Local rail */}
        <aside style={{
          background: "rgba(0,0,0,0.18)",
          borderRight: "1px solid rgba(255,255,255,0.05)",
          display: "flex", flexDirection: "column", minHeight: 0,
        }}>
          <WinLabel style={{ padding: "12px 14px 6px" }}>Local · 4</WinLabel>
          <div style={{ padding: "0 6px", display: "flex", flexDirection: "column", gap: 1 }}>
            {local.map((m) => {
              const active = m.id === selected;
              const tone = m.status === "loaded" ? "var(--success-400)" : m.status === "downloading" ? "var(--warning-400)" : "var(--fg-3)";
              return (
                <div key={m.id} style={{
                  padding: "9px 10px",
                  borderRadius: 6,
                  background: active ? "rgba(255,255,255,0.07)" : "transparent",
                  borderLeft: active ? "2px solid var(--brand-400)" : "2px solid transparent",
                  display: "flex", flexDirection: "column", gap: 3,
                  cursor: "pointer",
                }}>
                  <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                    <span style={{ width: 6, height: 6, borderRadius: "50%", background: tone, boxShadow: m.status === "loaded" ? `0 0 4px ${tone}` : "none" }} />
                    <span style={{ fontFamily: "var(--font-mono)", fontSize: 11.5, color: "var(--fg-0)", letterSpacing: "-0.005em" }}>{m.name}</span>
                  </div>
                  <div style={{ display: "flex", justifyContent: "space-between", fontSize: 10, color: "var(--fg-3)" }}>
                    <span>{m.family}</span>
                    <span style={{ fontFamily: "var(--font-mono)" }}>{m.size}</span>
                  </div>
                  {m.status === "downloading" && (
                    <div style={{ height: 2, background: "rgba(255,255,255,0.06)", borderRadius: 1, marginTop: 4, overflow: "hidden" }}>
                      <div style={{ width: "62%", height: "100%", background: "var(--warning-400)" }} />
                    </div>
                  )}
                </div>
              );
            })}
          </div>
          <div style={{ flex: 1 }} />
          <div style={{ padding: "10px 12px", borderTop: "1px solid rgba(255,255,255,0.05)", fontSize: 10.5, color: "var(--fg-3)", lineHeight: 1.5 }}>
            Right-click for pin · open in chat · delete.
          </div>
        </aside>

        {/* Search results */}
        <main style={{ display: "flex", flexDirection: "column", minHeight: 0 }}>
          <div style={{ padding: "14px 18px 10px", display: "flex", flexDirection: "column", gap: 10 }}>
            <div style={{
              display: "flex", alignItems: "center", gap: 9,
              height: 32, padding: "0 12px",
              background: "rgba(255,255,255,0.04)",
              border: "1px solid rgba(255,255,255,0.07)",
              borderRadius: 8,
            }}>
              <i className="fa-solid fa-magnifying-glass" style={{ fontSize: 11, color: "var(--fg-3)" }} />
              <span style={{ fontSize: 12.5, color: "var(--fg-1)" }}>coder · gguf · q4_k_m</span>
              <div style={{ flex: 1 }} />
              <span style={{ fontFamily: "var(--font-mono)", fontSize: 10, color: "var(--fg-3)" }}>5 results · huggingface.co</span>
            </div>
            <div style={{ display: "flex", gap: 6, flexWrap: "wrap" }}>
              {["Gemma", "Llama", "Phi", "Qwen", "Mistral", "≤ 5 GB", "≤ 10 GB", "Has vision", "Has tools"].map((f, i) => (
                <span key={f} style={{
                  fontSize: 10.5, padding: "3px 9px", borderRadius: 999,
                  background: i < 2 ? "rgba(64,193,197,0.10)" : "rgba(255,255,255,0.04)",
                  border: i < 2 ? "1px solid rgba(64,193,197,0.20)" : "1px solid rgba(255,255,255,0.06)",
                  color: i < 2 ? "var(--brand-300)" : "var(--fg-2)",
                  letterSpacing: "-0.005em",
                }}>{f}</span>
              ))}
            </div>
          </div>
          <div style={{ flex: 1, overflow: "auto", padding: "4px 18px 18px", display: "flex", flexDirection: "column", gap: 8 }}>
            {results.map((r, i) => (
              <div key={r.name} style={{
                padding: "12px 14px",
                borderRadius: 8,
                background: i === 0 ? "rgba(255,255,255,0.05)" : "rgba(255,255,255,0.025)",
                border: i === 0 ? "1px solid rgba(64,193,197,0.22)" : "1px solid rgba(255,255,255,0.05)",
                display: "flex", alignItems: "center", gap: 14,
              }}>
                <div style={{ flex: 1, minWidth: 0 }}>
                  <div style={{ fontFamily: "var(--font-mono)", fontSize: 12, color: "var(--fg-0)", letterSpacing: "-0.005em" }}>{r.name}</div>
                  <div style={{ fontSize: 10.5, color: "var(--fg-3)", marginTop: 3, display: "flex", gap: 12 }}>
                    <span>by {r.author}</span>
                    <span>· {r.downloads} downloads</span>
                  </div>
                  <div style={{ display: "flex", gap: 4, marginTop: 6 }}>
                    {[r.family, r.q, r.tools && "tools", r.vision && "vision"].filter(Boolean).map((b) => (
                      <span key={b} style={{
                        fontFamily: "var(--font-mono)", fontSize: 9.5,
                        padding: "1px 6px", borderRadius: 999,
                        background: "rgba(255,255,255,0.05)",
                        border: "1px solid rgba(255,255,255,0.07)",
                        color: "var(--fg-2)", letterSpacing: "0.02em",
                      }}>{b}</span>
                    ))}
                  </div>
                </div>
                <div style={{ textAlign: "right", fontFamily: "var(--font-mono)", fontSize: 11.5, color: "var(--fg-1)" }}>{r.size}</div>
                <WinBtn tone={i === 0 ? "primary" : "ghost"} size="sm"
                  icon={<i className="fa-solid fa-arrow-down" style={{ fontSize: 10 }} />}>
                  Download
                </WinBtn>
              </div>
            ))}
          </div>
        </main>

        {/* Detail */}
        <aside style={{
          background: "rgba(0,0,0,0.18)",
          borderLeft: "1px solid rgba(255,255,255,0.05)",
          padding: 18, overflow: "auto",
          display: "flex", flexDirection: "column", gap: 14,
        }}>
          <div>
            <WinLabel>Selected</WinLabel>
            <div style={{ fontFamily: "var(--font-mono)", fontSize: 13, color: "var(--fg-0)", marginTop: 6, letterSpacing: "-0.005em" }}>gemma-4-e2b</div>
            <div style={{ fontSize: 11, color: "var(--fg-3)", marginTop: 3 }}>by Google · loaded · 2.1 GB on disk</div>
          </div>
          <div style={{ display: "flex", gap: 6 }}>
            <WinBtn tone="primary" size="md" style={{ flex: 1, justifyContent: "center" }}
              icon={<i className="fa-regular fa-comment" />}>Open in chat</WinBtn>
            <WinBtn tone="ghost" size="md"
              icon={<i className="fa-solid fa-thumbtack" style={{ fontSize: 10 }} />} />
          </div>
          <div style={{ display: "flex", flexDirection: "column", gap: 8, fontSize: 11.5 }}>
            <RailRow k="Family"        v="Gemma 4" />
            <RailRow k="Parameters"    v="2 B" />
            <RailRow k="Quantisation"  v="q4_k_m" />
            <RailRow k="Context"       v="8,192" />
            <RailRow k="Vocabulary"    v="262,144" />
            <RailRow k="Architecture"  v="MoE · 4-expert" />
            <RailRow k="Last loaded"   v="2 minutes ago" />
            <RailRow k="Average tok/s" v="47.2 · last 100 runs" />
          </div>
          <div style={{
            padding: 10, borderRadius: 6,
            background: "rgba(255,255,255,0.03)",
            border: "1px solid rgba(255,255,255,0.06)",
          }}>
            <WinLabel>Files</WinLabel>
            <div style={{ marginTop: 6, display: "flex", flexDirection: "column", gap: 4, fontFamily: "var(--font-mono)", fontSize: 10.5, color: "var(--fg-2)" }}>
              <div style={{ display: "flex", justifyContent: "space-between" }}><span>gemma-4-e2b-q4_k_m.gguf</span><span style={{ color: "var(--fg-3)" }}>1.9 GB</span></div>
              <div style={{ display: "flex", justifyContent: "space-between" }}><span>tokenizer.json</span><span style={{ color: "var(--fg-3)" }}>4.2 MB</span></div>
              <div style={{ display: "flex", justifyContent: "space-between" }}><span>config.json</span><span style={{ color: "var(--fg-3)" }}>1.1 KB</span></div>
            </div>
          </div>
          <div style={{ fontSize: 11, color: "var(--fg-3)", lineHeight: 1.55 }}>
            Small dense model tuned for assistant-style turns. Lethean-recommended
            starter — fastest tok/s per watt on Apple Silicon at this size.
          </div>
        </aside>
      </div>
    </LthnWindow>
  );
}

Object.assign(window, {
  WelcomeWindow, SettingsWindow, ModelBrowserWindow,
});
