import { execFileSync } from "node:child_process";
import { createRequire } from "node:module";
import fs from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

import postcss from "postcss";
import tailwindPostcss from "@tailwindcss/postcss";

const require = createRequire(import.meta.url);
const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const webDir = path.resolve(__dirname, "..");
const repoRoot = path.resolve(webDir, "..");
const distDir = path.join(webDir, "dist");
const distAssetsDir = path.join(distDir, "assets");
const backendStaticDir = path.join(repoRoot, "backend", "static");
const publicDir = path.join(webDir, "public");
const mainJs = path.join(distAssetsDir, "main.js");
const mainCss = path.join(distAssetsDir, "main.css");

function getEsbuildBinary() {
  const platform = process.platform;
  const arch = process.arch;
  const packageNameByPlatform = {
    win32: {
      x64: "@esbuild/win32-x64",
      arm64: "@esbuild/win32-arm64",
      ia32: "@esbuild/win32-ia32",
    },
    linux: {
      x64: "@esbuild/linux-x64",
      arm64: "@esbuild/linux-arm64",
      arm: "@esbuild/linux-arm",
      ia32: "@esbuild/linux-ia32",
      ppc64: "@esbuild/linux-ppc64",
      s390x: "@esbuild/linux-s390x",
      loong64: "@esbuild/linux-loong64",
      riscv64: "@esbuild/linux-riscv64",
      mips64el: "@esbuild/linux-mips64el",
    },
    darwin: {
      x64: "@esbuild/darwin-x64",
      arm64: "@esbuild/darwin-arm64",
    },
    freebsd: {
      x64: "@esbuild/freebsd-x64",
      arm64: "@esbuild/freebsd-arm64",
    },
    openbsd: {
      x64: "@esbuild/openbsd-x64",
      arm64: "@esbuild/openbsd-arm64",
    },
    netbsd: {
      x64: "@esbuild/netbsd-x64",
      arm64: "@esbuild/netbsd-arm64",
    },
    sunos: {
      x64: "@esbuild/sunos-x64",
    },
    aix: {
      ppc64: "@esbuild/aix-ppc64",
    },
    android: {
      arm: "@esbuild/android-arm",
      arm64: "@esbuild/android-arm64",
      x64: "@esbuild/android-x64",
    },
    ohos: {
      arm64: "@esbuild/openharmony-arm64",
    },
  };

  const packageName = packageNameByPlatform[platform]?.[arch];
  if (!packageName) {
    throw new Error(`Unsupported esbuild platform: ${platform}-${arch}`);
  }

  const packageDir = path.dirname(require.resolve(`${packageName}/package.json`));
  const binaryName = platform === "win32" ? "esbuild.exe" : path.join("bin", "esbuild");
  const binaryPath = path.join(packageDir, binaryName);
  return binaryPath;
}

async function copyPublicAssets() {
  if (await exists(publicDir)) {
    await fs.cp(publicDir, distDir, { recursive: true, force: true });
  }
}

async function writeIndexHtml() {
  const assetVersion = Date.now();
  const html = `<!doctype html>
<html lang="zh-CN">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>ChatGpt Image Studio</title>
    <meta name="description" content="Image workspace and account control center" />
    <link rel="icon" type="image/svg+xml" href="/favicon-studio.svg" />
    <link rel="stylesheet" href="/assets/main.css?v=${assetVersion}" />
    <script type="module" src="/assets/main.js?v=${assetVersion}"></script>
  </head>
  <body class="antialiased">
    <div id="root"></div>
  </body>
</html>
`;
  await fs.writeFile(path.join(distDir, "index.html"), html);
}

async function processCss() {
  const cssPath = mainCss;
  const input = await fs.readFile(cssPath, "utf8");
  const result = await postcss([tailwindPostcss]).process(input, { from: cssPath, to: cssPath });
  await fs.writeFile(cssPath, result.css);
}

async function exists(targetPath) {
  try {
    await fs.access(targetPath);
    return true;
  } catch {
    return false;
  }
}

async function main() {
  await fs.rm(distDir, { recursive: true, force: true });
  await fs.mkdir(distAssetsDir, { recursive: true });

  const esbuildBinary = getEsbuildBinary();
  execFileSync(
    esbuildBinary,
    [
      "src/main.tsx",
      "--bundle",
      "--format=esm",
      "--platform=browser",
      "--conditions=style",
      "--tsconfig=tsconfig.json",
      `--outfile=${path.relative(webDir, mainJs)}`,
      "--loader:.svg=file",
      "--loader:.ico=file",
      "--loader:.png=file",
      "--loader:.jpg=file",
      "--loader:.jpeg=file",
      "--loader:.gif=file",
      "--loader:.webp=file",
      "--loader:.css=css",
      "--define:import.meta.env.DEV=false",
      "--define:import.meta.env.PROD=true",
    ],
    {
      cwd: webDir,
      stdio: "inherit",
    },
  );

  await processCss();
  await copyPublicAssets();
  await writeIndexHtml();

  await fs.rm(backendStaticDir, { recursive: true, force: true });
  await fs.mkdir(backendStaticDir, { recursive: true });
  await fs.cp(distDir, backendStaticDir, { recursive: true });

  console.log(`[build-static] synced ${distDir} -> ${backendStaticDir}`);
}

main().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
