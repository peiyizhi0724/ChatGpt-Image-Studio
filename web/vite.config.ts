import path from "node:path";
import { fileURLToPath } from "node:url";
import fs from "node:fs/promises";

import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig, type PluginOption } from "vite";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const backendStaticDir = path.resolve(__dirname, "../backend/static");

function syncBackendStaticPlugin(): PluginOption {
  let outDir = path.resolve(__dirname, "dist");

  return {
    name: "sync-backend-static",
    apply: "build",
    configResolved(config) {
      outDir = path.resolve(config.root, config.build.outDir);
    },
    async writeBundle() {
      await fs.rm(backendStaticDir, { recursive: true, force: true });
      await fs.mkdir(backendStaticDir, { recursive: true });
      await fs.cp(outDir, backendStaticDir, { recursive: true });
      console.log(`[vite] synced ${outDir} -> ${backendStaticDir}`);
    },
  };
}

export default defineConfig({
  plugins: [react(), tailwindcss(), syncBackendStaticPlugin()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  build: {
    outDir: "dist",
  },
});
