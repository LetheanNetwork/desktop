/* windows-chat.jsx — E0 · Chat window
 * Full-conversation interface · 1100×740 default, 900×600 min.
 * Layout: conversation rail (240) · conversation surface (flex) · optional right rail (280)
 * State variants: empty, mid-generation, multi-turn, switched-model warning, model-not-loaded
 *
 * Source-of-truth dependency: the runner state lives in the tray panel.
 * This window is a view + composer; it issues prompts to the same
 * runner process and shares any in-flight generation with the popover.
 */

/* ── Conversation rail (left, 240px) ─────────────────────────────── */
function ChatRail({ conversations, activeId, empty = false }) {
  return (
    <aside style={{
      width: 240, flexShrink: 0,
      borderRight: "1px solid rgba(255,255,255,0.05)",
      background: "rgba(0,0,0,0.18)",
      display: "flex", flexDirection: "column",
      minHeight: 0,
    }}>
      {/* Search */}
      <div style={{ padding: "12px 12px 8px" }}>
        <div style={{
          display: "flex", alignItems: "center", gap: 7,
          height: 28, padding: "0 10px",
          background: "rgba(255,255,255,0.04)",
          border: "1px solid rgba(255,255,255,0.06)",
          borderRadius: 6,
        }}>
          <i className="fa-solid fa-magnifying-glass" style={{ fontSize: 10, color: "var(--fg-3)" }} />
          <span style={{ fontSize: 11.5, color: "var(--fg-3)" }}>Search conversations</span>
          <div style={{ flex: 1 }} />
          <span style={{
            fontFamily: "var(--font-mono)", fontSize: 9.5,
            color: "var(--fg-3)", padding: "1px 4px",
            border: "1px solid rgba(255,255,255,0.08)", borderRadius: 3,
          }}>⌘K</span>
        </div>
      </div>

      {/* List */}
      <div style={{ flex: 1, minHeight: 0, overflow: "hidden", padding: "0 6px 8px" }}>
        {empty ? (
          <div style={{
            padding: "60px 16px 0", textAlign: "center",
            fontSize: 11.5, color: "var(--fg-3)", lineHeight: 1.55,
          }}>
            <div style={{
              width: 36, height: 36, margin: "0 auto 12px",
              borderRadius: 8, background: "rgba(255,255,255,0.04)",
              display: "flex", alignItems: "center", justifyContent: "center",
            }}>
              <i className="fa-regular fa-comment" style={{ fontSize: 14, color: "var(--fg-3)" }} />
            </div>
            No conversations yet.<br />
            Start one from the composer.
          </div>
        ) : (
          <div style={{ display: "flex", flexDirection: "column", gap: 1 }}>
            {/* Today group */}
            <WinLabel style={{ padding: "8px 8px 4px" }}>Today</WinLabel>
            {(conversations || []).filter(c => c.bucket === "today").map((c) => (
              <ChatRailItem key={c.id} {...c} active={c.id === activeId} />
            ))}
            <WinLabel style={{ padding: "12px 8px 4px" }}>Yesterday</WinLabel>
            {(conversations || []).filter(c => c.bucket === "yesterday").map((c) => (
              <ChatRailItem key={c.id} {...c} active={c.id === activeId} />
            ))}
            <WinLabel style={{ padding: "12px 8px 4px" }}>This week</WinLabel>
            {(conversations || []).filter(c => c.bucket === "week").map((c) => (
              <ChatRailItem key={c.id} {...c} active={c.id === activeId} />
            ))}
          </div>
        )}
      </div>

      {/* Rail footer — new conversation */}
      <div style={{
        padding: "8px 10px",
        borderTop: "1px solid rgba(255,255,255,0.05)",
        display: "flex", gap: 6,
      }}>
        <WinBtn tone="ghost" size="md" style={{ flex: 1, justifyContent: "center" }}
          icon={<i className="fa-solid fa-plus" style={{ fontSize: 10 }} />}>
          New conversation
        </WinBtn>
        <WinBtn tone="quiet" size="md" icon={<i className="fa-solid fa-ellipsis" style={{ fontSize: 11 }} />} />
      </div>
    </aside>
  );
}

function ChatRailItem({ title, model, snippet, active }) {
  return (
    <div style={{
      padding: "8px 10px",
      borderRadius: 6,
      background: active ? "rgba(255,255,255,0.07)" : "transparent",
      borderLeft: active ? "2px solid var(--brand-400)" : "2px solid transparent",
      display: "flex", flexDirection: "column", gap: 2,
      cursor: "pointer",
    }}>
      <div style={{
        fontSize: 12, fontWeight: 500, color: "var(--fg-0)",
        whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis",
        letterSpacing: "-0.005em",
      }}>{title}</div>
      <div style={{
        fontSize: 10.5, color: "var(--fg-3)",
        whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis",
      }}>{snippet}</div>
      <div style={{
        fontFamily: "var(--font-mono)", fontSize: 9, color: "var(--fg-3)",
        letterSpacing: "0.02em", marginTop: 1,
      }}>{model}</div>
    </div>
  );
}

/* ── Composer ────────────────────────────────────────────────────── */
function ChatComposer({ value, disabled, sending, error, hint }) {
  return (
    <div style={{
      padding: "14px 22px 16px",
      borderTop: "1px solid rgba(255,255,255,0.05)",
      background: "rgba(0,0,0,0.12)",
      display: "flex", flexDirection: "column", gap: 8,
    }}>
      {error && (
        <div style={{
          display: "flex", alignItems: "center", gap: 8,
          padding: "7px 11px",
          background: "rgba(255,76,76,0.08)",
          border: "1px solid rgba(255,76,76,0.18)",
          borderRadius: 6,
          fontSize: 11.5, color: "var(--danger-300)",
        }}>
          <i className="fa-solid fa-triangle-exclamation" style={{ fontSize: 11 }} />
          {error}
        </div>
      )}
      <div style={{
        position: "relative",
        background: disabled ? "rgba(255,255,255,0.02)" : "rgba(255,255,255,0.04)",
        border: "1px solid rgba(255,255,255,0.07)",
        borderRadius: 10,
        minHeight: 78,
        padding: "12px 14px 38px",
        opacity: disabled ? 0.55 : 1,
      }}>
        <div style={{
          fontSize: 13, lineHeight: 1.5,
          color: value ? "var(--fg-0)" : "var(--fg-3)",
          fontFamily: "var(--font-sans)",
          whiteSpace: "pre-wrap",
        }}>{value || (disabled ? "Load a model from the tray to start composing." : "Ask anything — runs locally on this Mac.")}</div>

        {/* Composer chrome row */}
        <div style={{
          position: "absolute", left: 12, right: 12, bottom: 8,
          display: "flex", alignItems: "center", gap: 8,
        }}>
          <WinBtn tone="quiet" size="sm" dim={disabled}
            icon={<i className="fa-regular fa-paperclip" style={{ fontSize: 11 }} />}>
            Attach
          </WinBtn>
          <WinBtn tone="quiet" size="sm" dim={disabled}
            icon={<i className="fa-solid fa-slash-forward" style={{ fontSize: 10 }} />}>
            Slash commands
          </WinBtn>
          <div style={{ flex: 1 }} />
          {hint && (
            <span style={{
              fontFamily: "var(--font-mono)", fontSize: 10,
              color: "var(--fg-3)", letterSpacing: "0.02em",
            }}>{hint}</span>
          )}
          {sending ? (
            <WinBtn tone="danger" size="sm"
              icon={<i className="fa-solid fa-stop" style={{ fontSize: 10 }} />}>
              Stop
            </WinBtn>
          ) : (
            <WinBtn tone="primary" size="sm" dim={disabled}
              icon={<i className="fa-solid fa-arrow-up" style={{ fontSize: 11 }} />}>
              Send
            </WinBtn>
          )}
        </div>
      </div>
    </div>
  );
}

/* ── Turn renderers ──────────────────────────────────────────────── */
function ChatTurn({ role = "you", text, code, streaming = false, citations }) {
  const isYou = role === "you";
  return (
    <div style={{
      display: "flex", flexDirection: "column", gap: 8,
      paddingTop: 4,
    }}>
      <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
        <div style={{
          width: 22, height: 22, borderRadius: 6,
          background: isYou
            ? "linear-gradient(145deg, rgba(255,255,255,0.10), rgba(255,255,255,0.04))"
            : "linear-gradient(145deg, rgba(64,193,197,0.30), rgba(64,193,197,0.08))",
          border: "1px solid rgba(255,255,255,0.06)",
          display: "flex", alignItems: "center", justifyContent: "center",
        }}>
          {isYou ? (
            <i className="fa-solid fa-user" style={{ fontSize: 10, color: "var(--fg-1)" }} />
          ) : (
            <LthnGlyph size={12} color="var(--brand-200)" />
          )}
        </div>
        <div style={{
          fontSize: 12, fontWeight: 600, color: "var(--fg-0)",
          letterSpacing: "-0.005em",
        }}>{isYou ? "You" : "Gemma 4 E2B"}</div>
        <div style={{ flex: 1 }} />
        {!streaming && (
          <div style={{ display: "flex", gap: 4, opacity: 0.5 }}>
            <WinBtn tone="quiet" size="sm"
              icon={<i className="fa-regular fa-copy" style={{ fontSize: 10 }} />} />
            <WinBtn tone="quiet" size="sm"
              icon={<i className="fa-solid fa-rotate-right" style={{ fontSize: 10 }} />} />
          </div>
        )}
      </div>
      <div style={{
        marginLeft: 30,
        fontSize: 13.5, lineHeight: 1.6,
        color: "var(--fg-1)",
        whiteSpace: "pre-wrap",
        letterSpacing: "-0.003em",
      }}>
        {text}
        {streaming && <span style={{
          display: "inline-block", width: 7, height: 14,
          background: "var(--brand-300)", verticalAlign: "-3px",
          marginLeft: 2, animation: "lthn-cursor 1s steps(2) infinite",
        }} />}
      </div>
      {code && (
        <div style={{
          marginLeft: 30,
          borderRadius: 8,
          background: "rgba(0,0,0,0.32)",
          border: "1px solid rgba(255,255,255,0.05)",
          overflow: "hidden",
        }}>
          <div style={{
            display: "flex", alignItems: "center", gap: 8,
            padding: "6px 12px",
            borderBottom: "1px solid rgba(255,255,255,0.04)",
            background: "rgba(0,0,0,0.18)",
          }}>
            <span style={{
              fontFamily: "var(--font-mono)", fontSize: 10,
              color: "var(--fg-3)", letterSpacing: "0.04em", textTransform: "uppercase",
            }}>{code.lang}</span>
            <div style={{ flex: 1 }} />
            <WinBtn tone="quiet" size="sm"
              icon={<i className="fa-regular fa-copy" style={{ fontSize: 10 }} />}>
              Copy
            </WinBtn>
          </div>
          <pre style={{
            margin: 0, padding: "12px 14px",
            fontFamily: "var(--font-mono)", fontSize: 11.5,
            lineHeight: 1.6, color: "var(--fg-1)",
            whiteSpace: "pre", overflow: "auto",
          }}>{code.text}</pre>
        </div>
      )}
      {citations && (
        <div style={{ marginLeft: 30, display: "flex", flexWrap: "wrap", gap: 6, marginTop: 2 }}>
          {citations.map((c, i) => (
            <span key={i} style={{
              fontFamily: "var(--font-mono)", fontSize: 10,
              color: "var(--brand-300)",
              padding: "2px 7px",
              borderRadius: 999,
              background: "rgba(64,193,197,0.08)",
              border: "1px solid rgba(64,193,197,0.18)",
            }}><i className="fa-solid fa-link" style={{ fontSize: 8, marginRight: 4 }} />{c}</span>
          ))}
        </div>
      )}
    </div>
  );
}

/* ── Conversation surface ────────────────────────────────────────── */
function ChatSurface({ turns, banner, empty, streamingIdx, scrollNotice }) {
  return (
    <main style={{
      flex: 1, minHeight: 0,
      display: "flex", flexDirection: "column",
      position: "relative",
    }}>
      {banner && (
        <div style={{
          flexShrink: 0,
          margin: "12px 22px 0",
          padding: "10px 12px",
          borderRadius: 8,
          background: banner.tone === "warn"
            ? "rgba(217,154,72,0.08)"
            : "rgba(64,193,197,0.06)",
          border: `1px solid ${banner.tone === "warn" ? "rgba(217,154,72,0.20)" : "rgba(64,193,197,0.18)"}`,
          display: "flex", alignItems: "center", gap: 10,
          fontSize: 11.5, color: banner.tone === "warn" ? "var(--warning-300)" : "var(--brand-200)",
        }}>
          <i className={banner.tone === "warn" ? "fa-solid fa-circle-exclamation" : "fa-solid fa-circle-info"}
            style={{ fontSize: 11 }} />
          <div style={{ flex: 1, lineHeight: 1.5 }}>{banner.text}</div>
          {banner.action && (
            <WinBtn tone="ghost" size="sm">{banner.action}</WinBtn>
          )}
        </div>
      )}

      <div style={{
        flex: 1, minHeight: 0, overflow: "hidden",
        padding: "20px 22px",
        display: "flex", flexDirection: "column", gap: 22,
      }}>
        {empty ? (
          <ChatEmpty />
        ) : (
          (turns || []).map((t, i) => (
            <ChatTurn key={i} {...t} streaming={i === streamingIdx} />
          ))
        )}
      </div>

      {scrollNotice && (
        <div style={{
          position: "absolute", bottom: 14, right: 22,
          padding: "6px 11px", borderRadius: 999,
          background: "rgba(255,255,255,0.08)",
          backdropFilter: "blur(20px)",
          border: "1px solid rgba(255,255,255,0.10)",
          fontSize: 11, color: "var(--fg-1)",
          display: "flex", alignItems: "center", gap: 6,
        }}>
          <i className="fa-solid fa-arrow-down" style={{ fontSize: 10 }} />
          {scrollNotice}
        </div>
      )}
    </main>
  );
}

/* Empty / first-conversation hero */
function ChatEmpty() {
  const starters = [
    { icon: "fa-code", text: "Explain this regex" },
    { icon: "fa-pen-nib", text: "Rewrite a paragraph" },
    { icon: "fa-table", text: "Reshape this JSON" },
    { icon: "fa-question", text: "What can this model do?" },
  ];
  return (
    <div style={{
      flex: 1,
      display: "flex", flexDirection: "column",
      alignItems: "center", justifyContent: "center",
      gap: 22, padding: 40,
    }}>
      <div style={{
        width: 56, height: 56, borderRadius: 14,
        background: "linear-gradient(155deg, rgba(64,193,197,0.18), rgba(64,193,197,0.02))",
        border: "1px solid rgba(64,193,197,0.18)",
        display: "flex", alignItems: "center", justifyContent: "center",
      }}>
        <LthnGlyph size={28} color="var(--brand-200)" active={true} />
      </div>
      <div style={{ textAlign: "center", display: "flex", flexDirection: "column", gap: 8 }}>
        <div style={{ fontSize: 22, fontWeight: 600, color: "var(--fg-0)", letterSpacing: "-0.015em" }}>
          What shall we look at?
        </div>
        <div style={{ fontSize: 13, color: "var(--fg-2)", maxWidth: 420, lineHeight: 1.55 }}>
          Conversations stay on this Mac. Nothing leaves unless you flip
          on the API server in Settings and a client connects.
        </div>
      </div>
      <div style={{
        display: "grid", gridTemplateColumns: "repeat(2, 220px)", gap: 8,
        marginTop: 6,
      }}>
        {starters.map((s) => (
          <div key={s.text} style={{
            padding: "10px 12px",
            borderRadius: 8,
            background: "rgba(255,255,255,0.03)",
            border: "1px solid rgba(255,255,255,0.06)",
            display: "flex", alignItems: "center", gap: 10,
            fontSize: 12, color: "var(--fg-1)",
            cursor: "pointer",
          }}>
            <i className={`fa-solid ${s.icon}`} style={{ fontSize: 11, color: "var(--fg-3)" }} />
            {s.text}
          </div>
        ))}
      </div>
    </div>
  );
}

/* ── Right rail · turn metadata ──────────────────────────────────── */
function ChatRightRail({ data, collapsed = false }) {
  if (collapsed) {
    return (
      <aside style={{
        width: 36, flexShrink: 0,
        borderLeft: "1px solid rgba(255,255,255,0.05)",
        background: "rgba(0,0,0,0.18)",
        display: "flex", flexDirection: "column",
        alignItems: "center", padding: "10px 0", gap: 10,
      }}>
        <WinBtn tone="quiet" size="sm"
          icon={<i className="fa-solid fa-chart-line" style={{ fontSize: 11 }} />} />
        <WinBtn tone="quiet" size="sm"
          icon={<i className="fa-solid fa-bolt" style={{ fontSize: 11 }} />} />
        <WinBtn tone="quiet" size="sm"
          icon={<i className="fa-solid fa-database" style={{ fontSize: 11 }} />} />
      </aside>
    );
  }
  return (
    <aside style={{
      width: 280, flexShrink: 0,
      borderLeft: "1px solid rgba(255,255,255,0.05)",
      background: "rgba(0,0,0,0.18)",
      display: "flex", flexDirection: "column",
      minHeight: 0, overflow: "hidden",
    }}>
      <div style={{
        padding: "14px 18px 8px",
        display: "flex", alignItems: "center", gap: 8,
      }}>
        <WinLabel>Turn metadata</WinLabel>
        <div style={{ flex: 1 }} />
        <WinBtn tone="quiet" size="sm"
          icon={<i className="fa-solid fa-angle-right" style={{ fontSize: 10 }} />} />
      </div>
      <div style={{ padding: "0 18px 16px", display: "flex", flexDirection: "column", gap: 14, overflow: "auto" }}>
        {/* Live tok/s */}
        <RailStat label="Tok/s · live" value={data.toksLive || "—"} sparkline={data.sparkline} />
        <RailStat label="Watts · this turn" value={data.watts || "—"} />
        <RailStat label="KV cache hit" value={data.kvHit || "—"} />
        <RailStat label="Tokens used" value={data.tokens || "—"} />

        <WinLabel style={{ marginTop: 4 }}>Sampling</WinLabel>
        <div style={{ display: "flex", flexDirection: "column", gap: 6, fontSize: 11.5 }}>
          <RailRow k="Temperature" v="0.7" />
          <RailRow k="Top-p" v="0.95" />
          <RailRow k="Max tokens" v="1024" />
          <RailRow k="Context" v={data.ctx || "—"} />
        </div>

        <WinLabel style={{ marginTop: 4 }}>Sources</WinLabel>
        {data.sources ? data.sources.map((s, i) => (
          <div key={i} style={{
            padding: "8px 10px",
            borderRadius: 6,
            background: "rgba(255,255,255,0.03)",
            border: "1px solid rgba(255,255,255,0.06)",
            fontSize: 11, color: "var(--fg-2)",
            display: "flex", flexDirection: "column", gap: 3,
          }}>
            <div style={{ color: "var(--fg-1)", fontWeight: 500 }}>{s.title}</div>
            <div style={{ color: "var(--fg-3)", fontSize: 10 }}>{s.kind}</div>
          </div>
        )) : (
          <div style={{ fontSize: 11, color: "var(--fg-3)", fontStyle: "italic" }}>
            None this turn. Citations appear here when the model grounds an answer.
          </div>
        )}
      </div>
    </aside>
  );
}

function RailStat({ label, value, sparkline }) {
  return (
    <div style={{
      padding: "10px 12px",
      borderRadius: 8,
      background: "rgba(255,255,255,0.03)",
      border: "1px solid rgba(255,255,255,0.06)",
      display: "flex", flexDirection: "column", gap: 4,
    }}>
      <div style={{
        fontFamily: "var(--font-mono)", fontSize: 9.5,
        color: "var(--fg-3)", letterSpacing: "0.06em", textTransform: "uppercase",
      }}>{label}</div>
      <div style={{ display: "flex", alignItems: "baseline", justifyContent: "space-between", gap: 6 }}>
        <div style={{
          fontFamily: "var(--font-mono)", fontSize: 19,
          color: "var(--fg-0)", letterSpacing: "-0.005em",
          fontWeight: 500,
        }}>{value}</div>
        {sparkline && <Sparkline width={70} height={22} />}
      </div>
    </div>
  );
}

function RailRow({ k, v }) {
  return (
    <div style={{ display: "flex", justifyContent: "space-between", gap: 12 }}>
      <span style={{ color: "var(--fg-3)" }}>{k}</span>
      <span style={{ fontFamily: "var(--font-mono)", color: "var(--fg-1)" }}>{v}</span>
    </div>
  );
}

/* ── Toolbar (model picker etc.) ─────────────────────────────────── */
function ChatToolbar({ model = "Gemma 4 E2B", ctx, tokens, rightRailOpen = true, onToggleRail }) {
  return (
    <>
      <div style={{
        display: "flex", alignItems: "center", gap: 6,
        padding: "4px 9px",
        borderRadius: 7,
        background: "rgba(255,255,255,0.04)",
        border: "1px solid rgba(255,255,255,0.07)",
        fontSize: 11.5, color: "var(--fg-1)",
      }}>
        <span style={{
          width: 5, height: 5, borderRadius: "50%",
          background: "var(--success-400)",
          boxShadow: "0 0 4px var(--success-400)",
        }} />
        {model}
        <i className="fa-solid fa-angle-down" style={{ fontSize: 9, color: "var(--fg-3)", marginLeft: 2 }} />
      </div>
      <div style={{
        fontFamily: "var(--font-mono)", fontSize: 10.5,
        color: "var(--fg-3)", letterSpacing: "0.02em",
        padding: "0 4px",
      }}>
        {ctx} · {tokens}
      </div>
      <WinBtn tone="ghost" size="sm"
        icon={<i className="fa-solid fa-sliders" style={{ fontSize: 10 }} />} />
      <WinBtn tone="ghost" size="sm" active={rightRailOpen}
        icon={<i className="fa-solid fa-chart-line" style={{ fontSize: 10 }} />} />
    </>
  );
}

/* ── The window — parameterised by state ──────────────────────────
 * states: "empty" | "generating" | "multi-turn" | "switched-model"
 *       | "no-model"
 */
function ChatWindow({ state = "multi-turn", rightRail = "expanded", rail = "filled", w = 1100, h = 740 }) {
  const conversations = [
    { id: "c1", bucket: "today",    title: "Refactor the embed loop",  snippet: "Looks like the issue is the closure capturing…", model: "Gemma 4 E2B" },
    { id: "c2", bucket: "today",    title: "Brief read · drone bill",  snippet: "Two paragraphs summarising the key clauses.",   model: "Llama 3.2 3B" },
    { id: "c3", bucket: "yesterday",title: "JSON to TOML",              snippet: "Here's the converted config in TOML…",         model: "Gemma 4 E2B" },
    { id: "c4", bucket: "yesterday",title: "Vi voice samples",          snippet: "Three drafts in plain-spoken register.",       model: "Gemma 4 E2B" },
    { id: "c5", bucket: "week",     title: "Tokeniser benchmarks",      snippet: "PP throughput on M3 Pro vs M4 Air…",           model: "Gemma 4 E2B" },
    { id: "c6", bucket: "week",     title: "Onboarding microcopy",      snippet: "Calm-presence voice across the welcome flow.", model: "Llama 3.2 3B" },
  ];

  const turnsMulti = [
    { role: "you", text: "Walk me through how this Go embed loop closes over its loop variable. The captured value is wrong on every iteration." },
    { role: "model", text: "The loop variable is shared across iterations — every closure captures the same address, so by the time the goroutines fire, they all see the final value.\n\nGo 1.22 changed this so each iteration gets its own copy. If you're on 1.21 or earlier, you need to shadow the variable explicitly.",
      code: { lang: "go", text: "for _, item := range items {\n    item := item // shadow for the closure\n    go func() {\n        process(item)\n    }()\n}" }
    },
    { role: "you", text: "Right, so just `item := item` before the goroutine. Why does the runtime not see this as a redundant assignment?" },
    { role: "model", text: "Because it isn't redundant — it creates a new variable in the inner scope. The compiler treats the inner `item` as a distinct binding; the closure captures THAT one. The optimiser can't elide it because the goroutine outlives the iteration.",
      citations: ["go.dev/ref/spec#Variable_scope", "go.dev/blog/loopvar-preview"] },
  ];

  const turnsGen = [
    ...turnsMulti.slice(0, 2),
    { role: "you", text: "Now write the test that would have caught this." },
    { role: "model", text: "A table-driven test that fires each goroutine and asserts the captured value matches the iteration — running it under `-race` proves the closure capture rather than just timing luck.\n\n" },
  ];

  // Decide rail content per state
  const railData = {
    empty:     { toksLive: "—", watts: "—", kvHit: "—", tokens: "—", ctx: "—" },
    generating:{ toksLive: "47.2", watts: "12.4 W", kvHit: "94%", tokens: "1,284 / 4,096", ctx: "1,284 / 4,096", sparkline: true, sources: [{ title: "Go specification · variable scope", kind: "Reference · loaded from cache" }] },
    "multi-turn":{ toksLive: "44.6", watts: "11.8 W", kvHit: "96%", tokens: "2,041 / 4,096", ctx: "2,041 / 4,096" },
    "switched-model":{ toksLive: "—", watts: "—", kvHit: "0%", tokens: "0 / 8,192", ctx: "0 / 8,192" },
    "no-model":{ toksLive: "—", watts: "—", kvHit: "—", tokens: "—", ctx: "—" },
  }[state];

  const turns = {
    empty: null,
    generating: turnsGen,
    "multi-turn": turnsMulti,
    "switched-model": turnsMulti,
    "no-model": null,
  }[state];

  const banner = state === "switched-model"
    ? { tone: "warn", text: "Switched to Llama 3.2 3B mid-conversation — KV cache cleared. The next turn will replay context.", action: "Restore Gemma" }
    : state === "no-model"
    ? { tone: "warn", text: "No model loaded. Pick one from the tray to start composing.", action: "Open tray" }
    : null;

  const composerProps = {
    empty: { value: "" },
    generating: { value: "Now write the test that would have caught this.", sending: true, hint: "Esc · stop" },
    "multi-turn": { value: "", hint: "⌘↵ · send" },
    "switched-model": { value: "", hint: "⌘↵ · send" },
    "no-model": { value: "", disabled: true },
  }[state];

  const toolbarModel = {
    empty: "Gemma 4 E2B",
    generating: "Gemma 4 E2B",
    "multi-turn": "Gemma 4 E2B",
    "switched-model": "Llama 3.2 3B",
    "no-model": "No model",
  }[state];

  const footer = state === "no-model"
    ? <><span style={{ width: 6, height: 6, borderRadius: "50%", background: "var(--fg-3)", display: "inline-block" }} />Idle · no model loaded</>
    : state === "generating"
    ? <><span style={{ width: 6, height: 6, borderRadius: "50%", background: "var(--success-400)", display: "inline-block", boxShadow: "0 0 4px var(--success-400)" }} />Generating · 47.2 t/s · 12.4 W · Airplane-mode OK</>
    : <><span style={{ width: 6, height: 6, borderRadius: "50%", background: "var(--success-400)", display: "inline-block" }} />Model ready · Airplane-mode OK · 1 runner · v0.2.0-rc1</>;

  return (
    <LthnWindow
      title="lthn · chat"
      subtitle="conversation · local"
      width={w}
      height={h}
      toolbar={
        <ChatToolbar
          model={toolbarModel}
          ctx={railData.ctx}
          tokens="1 runner"
          rightRailOpen={rightRail === "expanded"}
        />
      }
      footer={footer}
    >
      <div style={{ flex: 1, minHeight: 0, display: "flex" }}>
        <ChatRail conversations={conversations} activeId={state === "empty" ? null : "c1"} empty={rail === "empty"} />
        <div style={{ flex: 1, minWidth: 0, display: "flex", flexDirection: "column" }}>
          <ChatSurface
            empty={state === "empty"}
            turns={turns}
            streamingIdx={state === "generating" ? (turns?.length || 0) - 1 : -1}
            banner={banner}
          />
          <ChatComposer {...composerProps} />
        </div>
        {rightRail !== "hidden" && (
          <ChatRightRail data={railData} collapsed={rightRail === "collapsed"} />
        )}
      </div>
    </LthnWindow>
  );
}

Object.assign(window, {
  ChatWindow, ChatRail, ChatComposer, ChatSurface, ChatRightRail,
  ChatTurn, ChatEmpty, ChatToolbar, RailRow,
});
