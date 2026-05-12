import { defineConfig } from "vite";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";

const __dirname = dirname(fileURLToPath(import.meta.url));
const root = (p) => resolve(__dirname, p);

// Wails3 dev injects VITE_PORT=9245 via env (see root Taskfile.yml
// vars.VITE_PORT) and the Go binary connects to that URL via its
// AssetServer.DevServerURL config. Match the env or Wails sits in
// "Retrying..." until it gives up.
const port = Number(process.env.VITE_PORT) || 9245;

export default defineConfig({
  root: ".",
  publicDir: "public",
  // Path aliases — mirror tsconfig.json `paths`. Both must stay in
  // lockstep: tsc resolves at typecheck, Vite resolves at build.
  resolve: {
    alias: [
      { find: /^@\/(.*)$/,          replacement: root("src/$1") },
      { find: /^@ui\/(.*)$/,        replacement: root("src/lit/$1") },
      { find: /^@types$/,           replacement: root("src/lit/types.ts") },
      { find: /^@service$/,         replacement: root("bindings/dappco.re/lthn/desktop/pkg/desktop/index.ts") },
      { find: /^@service\/(.*)$/,   replacement: root("bindings/dappco.re/lthn/desktop/pkg/desktop/$1") },
      { find: /^@models\/(.*)$/,    replacement: root("bindings/dappco.re/lthn/desktop/pkg/$1/models.ts") },
      { find: /^@inference\/(.*)$/, replacement: root("bindings/dappco.re/go/inference/$1") },
      { find: /^@bindings\/(.*)$/,  replacement: root("bindings/$1") },
    ],
  },
  server: {
    port,
    strictPort: true,
    host: "localhost",
  },
  build: {
    outDir: "dist",
    emptyOutDir: true,
    target: "es2022",
    rollupOptions: {
      input: {
        index: "index.html",
        canvas: "canvas.html",
      },
    },
  },
});
