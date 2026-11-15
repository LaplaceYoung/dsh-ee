import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { fileURLToPath } from "node:url";

// 后端（server/）地址；同源代理让会话 cookie 直接可用
const backend = "http://127.0.0.1:8080";
const proxy = { "/api": backend, "/v1": backend, "/beta": backend, "/npm": backend, "/index": backend, "/setup.sh": backend, "/setup.ps1": backend };

export default defineConfig({
  plugins: [react(), tailwindcss()],
  // @workbench：接入插件里的工作台渲染（纯函数），网页版应用与 DSH 共用一套
  resolve: {
    alias: {
      "@": fileURLToPath(new URL("./src", import.meta.url)),
      "@workbench": fileURLToPath(new URL("../dsh-plugin/src", import.meta.url)),
    },
  },
  // 监听所有网卡，允许局域网 / 外部 IP 访问原型
  server: { host: "0.0.0.0", port: 5188, strictPort: true, allowedHosts: true, proxy, fs: { allow: [".."] } },
  preview: { host: "0.0.0.0", port: 5188, strictPort: true, allowedHosts: true, proxy },
});
