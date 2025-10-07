import react from "@vitejs/plugin-react";
import { defineConfig, loadEnv } from "vite";
import topLevelAwait from "vite-plugin-top-level-await";
import wasm from "vite-plugin-wasm";
import { resolve } from "path";

export default defineConfig(({ mode }) => {
  // Load env from root directory (parent of frontend/) for local dev
  const env = loadEnv(mode, resolve(__dirname, ".."), "");

  // Get LOG_LEVEL from process.env (Docker) or loaded env (local dev)
  const logLevel = process.env.LOG_LEVEL || env.LOG_LEVEL || "info";

  // Get VITE_SHA from process.env (Docker) or loaded env (local dev)
  const sha = process.env.VITE_SHA || env.VITE_SHA || undefined;

  return {
    base: "",
    build: {
      chunkSizeWarningLimit: 1000,
    },
    plugins: [wasm(), topLevelAwait(), react()],
    server: {
      proxy: {
        "/api": {
          target: "http://127.0.0.1:3030",
          changeOrigin: true,
          secure: false,
          ws: true,
        },
      },
    },
    define: {
      // Inject LOG_LEVEL as VITE_LOG_LEVEL (works in both Docker and local dev)
      "import.meta.env.VITE_LOG_LEVEL": JSON.stringify(logLevel),
      // Inject git SHA for version display (only if defined)
      ...(sha ? { "import.meta.env.VITE_SHA": JSON.stringify(sha) } : {}),
    },
  };
});
