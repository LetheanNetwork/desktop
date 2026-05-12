/* windows-chrome.jsx
 * Shared chrome for the Lethean Desktop expansion windows.
 * Pattern: macOS native window controls · MacTitleBarHiddenInsetUnified
 * (translucent backdrop, no separate title-bar strip; title sits inline
 * with content padding) · dark default · falls back to solid surface
 * for Linux/Windows (we render macOS chrome in these mocks since v0 is
 * macOS only).
 */

/* Traffic lights — non-interactive but full-fidelity */
function WinTrafficLights() {
  const dot = (bg, glyph) => (
    <div style={{
      width: 12, height: 12, borderRadius: "50%", background: bg,
      display: "flex", alignItems: "center", justifyContent: "center",
      boxShadow: "inset 0 0 0 0.5px rgba(0,0,0,0.18)",
    }}>{glyph}</div>
  );
  return (
    <div style={{ display: "flex", gap: 8, alignItems: "center" }}>
      {dot("#ff5f57")}{dot("#febc2e")}{dot("#28c840")}
    </div>
  );
}

/* The window shell.
 * Renders a translucent backdrop with the title inset; children fill
 * the body. `footer` is an optional pinned status bar (1 line, 28px).
 */
function LthnWindow({
  title = "Untitled",
  subtitle,                  // small mono-y suffix beside title
  width = 1100,
  height = 740,
  toolbar,                   // optional inline toolbar (right of title)
  footer,                    // optional bottom status bar (1 line)
  children,
  scale = 1,                 // overall scale (artboards sometimes need to fit larger windows)
}) {
  return (
    <div style={{
      width: width * scale,
      height: height * scale,
      transform: scale === 1 ? "none" : `scale(${scale})`,
      transformOrigin: "top left",
      display: "flex", flexDirection: "column",
      borderRadius: 12,
      overflow: "hidden",
      // Translucent backdrop (MacBackdropTranslucent)
      background: "linear-gradient(180deg, rgba(28,28,38,0.92), rgba(20,20,28,0.94))",
      backdropFilter: "blur(32px) saturate(160%)",
      WebkitBackdropFilter: "blur(32px) saturate(160%)",
      border: "1px solid rgba(255,255,255,0.07)",
      boxShadow: "0 30px 80px rgba(0,0,0,0.55), 0 12px 24px rgba(0,0,0,0.35), inset 0 1px 0 rgba(255,255,255,0.05)",
      fontFamily: "var(--font-sans)",
      color: "var(--fg-1)",
    }}>
      {/* Title bar — inset unified · no separate strip */}
      <div style={{
        flexShrink: 0,
        display: "flex", alignItems: "center", gap: 16,
        padding: "14px 18px 12px",
        borderBottom: "1px solid rgba(255,255,255,0.04)",
      }}>
        <WinTrafficLights />
        <div style={{ display: "flex", alignItems: "baseline", gap: 10, minWidth: 0 }}>
          <LthnGlyph size={14} color="var(--fg-1)" />
          <div style={{ fontSize: 13, fontWeight: 600, color: "var(--fg-0)", letterSpacing: "-0.005em" }}>{title}</div>
          {subtitle && (
            <div style={{
              fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--fg-3)",
              letterSpacing: "0.02em",
            }}>{subtitle}</div>
          )}
        </div>
        {toolbar && <div style={{ marginLeft: "auto", display: "flex", alignItems: "center", gap: 8 }}>{toolbar}</div>}
      </div>

      {/* Body */}
      <div style={{ flex: 1, minHeight: 0, display: "flex", flexDirection: "column", overflow: "hidden" }}>
        {children}
      </div>

      {/* Footer (optional) */}
      {footer && (
        <div style={{
          flexShrink: 0,
          height: 28,
          borderTop: "1px solid rgba(255,255,255,0.05)",
          background: "rgba(0,0,0,0.18)",
          display: "flex", alignItems: "center", gap: 12,
          padding: "0 14px",
          fontFamily: "var(--font-mono)", fontSize: 10.5,
          color: "var(--fg-3)", letterSpacing: "0.02em",
        }}>{footer}</div>
      )}
    </div>
  );
}

/* Compact button — primary / ghost / quiet */
function WinBtn({ tone = "ghost", icon, children, active = false, dim = false, size = "md", style = {}, onClick }) {
  const styles = {
    primary: {
      background: "var(--brand-500)", color: "#fff",
      border: "1px solid transparent",
    },
    ghost: {
      background: "rgba(255,255,255,0.05)",
      color: "var(--fg-1)",
      border: "1px solid rgba(255,255,255,0.07)",
    },
    quiet: {
      background: "transparent",
      color: "var(--fg-2)",
      border: "1px solid transparent",
    },
    danger: {
      background: "rgba(255,76,76,0.12)",
      color: "var(--danger-300)",
      border: "1px solid rgba(255,76,76,0.18)",
    },
  };
  const sizes = {
    sm: { padding: "4px 9px", fontSize: 11, gap: 5 },
    md: { padding: "6px 11px", fontSize: 12, gap: 6 },
    lg: { padding: "9px 16px", fontSize: 13, gap: 7 },
  };
  return (
    <button onClick={onClick} style={{
      display: "inline-flex", alignItems: "center",
      borderRadius: 6, cursor: "pointer",
      fontFamily: "var(--font-sans)", fontWeight: 500,
      letterSpacing: "-0.005em",
      opacity: dim ? 0.5 : 1,
      ...styles[tone], ...sizes[size],
      ...(active ? { background: "rgba(255,255,255,0.10)" } : {}),
      ...style,
    }}>
      {icon && <span style={{ display: "inline-flex" }}>{icon}</span>}
      {children}
    </button>
  );
}

/* Small caps section label */
function WinLabel({ children, style = {} }) {
  return (
    <div style={{
      fontFamily: "var(--font-mono)", fontSize: 9.5,
      color: "var(--fg-3)", letterSpacing: "0.1em",
      textTransform: "uppercase", ...style,
    }}>{children}</div>
  );
}

/* Frame around an artboard — gives every window mock the same "this is
 * sitting on a virtual desktop" matting. Optional context strip below
 * for short rationale notes. */
function WindowFrame({ name, blurb, children, bg = "linear-gradient(155deg, #0a0a10 0%, #11111a 100%)" }) {
  return (
    <div style={{
      width: "100%", height: "100%",
      background: bg,
      padding: 40,
      display: "flex", flexDirection: "column", gap: 18,
      alignItems: "center", justifyContent: "center",
      position: "relative",
    }}>
      {name && (
        <div style={{
          position: "absolute", top: 18, left: 24,
          fontFamily: "var(--font-mono)", fontSize: 10,
          color: "rgba(255,255,255,0.35)", letterSpacing: "0.08em",
          textTransform: "uppercase",
        }}>{name}</div>
      )}
      {children}
      {blurb && (
        <div style={{
          maxWidth: 720, textAlign: "center",
          fontSize: 12, lineHeight: 1.55,
          color: "rgba(255,255,255,0.5)",
        }}>{blurb}</div>
      )}
    </div>
  );
}

Object.assign(window, {
  WinTrafficLights, LthnWindow, WinBtn, WinLabel, WindowFrame,
});
