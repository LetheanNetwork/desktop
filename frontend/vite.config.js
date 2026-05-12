import { defineConfig } from "vite";

// Wails3 dev injects VITE_PORT=9245 via env (see root Taskfile.yml
// vars.VITE_PORT) and the Go binary connects to that URL via its
// AssetServer.DevServerURL config. Match the env or Wails sits in
// "Retrying..." until it gives up.
const port = Number(process.env.VITE_PORT) || 9245;

export default defineConfig({
  root: ".",
  publicDir: "public",
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
