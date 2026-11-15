import { build } from "esbuild";
import { execFileSync } from "node:child_process";

execFileSync("npx", ["tsc", "--noEmit"], { stdio: "inherit" });
await build({
  entryPoints: ["src/index.ts"],
  bundle: true,
  format: "esm",
  platform: "node",
  target: "node20",
  charset: "utf8",
  outfile: "../server/internal/api/connectpkg/index.js",
  banner: { js: "// 由 dsh-plugin/ 生成，勿手改。\n" },
});
await build({
  entryPoints: ["src/client.ts"],
  bundle: true,
  format: "cjs",
  platform: "browser",
  target: "es2022",
  charset: "utf8",
  external: ["react"],
  outfile: "../server/internal/api/connectpkg/client.js",
  banner: { js: '// 由 dsh-plugin/ 生成，勿手改。\nwindow.__ModuleLoader__.load({ id: "__DSH_EE_PACKAGE__", factory: (require) => {\nvar module = { exports: {} }; var exports = module.exports;' },
  footer: { js: "return module.exports; } });" },
});
