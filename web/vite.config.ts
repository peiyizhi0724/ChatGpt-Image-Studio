import path from "node:path";
import { fileURLToPath } from "node:url";
import fs from "node:fs/promises";

import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig, loadEnv, type PluginOption } from "vite";

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

const proxyPaths = ["/api", "/portal/api", "/v1", "/auth", "/version", "/health"];

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), "");
  const proxyTarget = env.VITE_API_PROXY_TARGET?.trim().replace(/\/$/, "");
  const proxy = proxyTarget
    ? Object.fromEntries(
        proxyPaths.map((pathname) => [
          pathname,
          {
            target: proxyTarget,
            changeOrigin: true,
            secure: false,
          },
        ]),
      )
    : undefined;

  return {
    plugins: [react(), tailwindcss(), syncBackendStaticPlugin()],
    base: "/admin/",
    resolve: {
      alias: {
        "@": path.resolve(__dirname, "./src"),
      },
    },
    build: {
      outDir: "dist",
    },
    server: proxy
      ? {
          proxy,
        }
      : undefined,
  };
});
