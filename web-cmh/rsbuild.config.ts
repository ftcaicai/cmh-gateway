// web-cmh build config. Mirrors the official web/rsbuild.config.ts shape
// (ext/AGENTS.md 搂6.2) but is an independent app served by ext at /cmh/.
import path from "node:path";
import { fileURLToPath } from "node:url";

import { defineConfig } from "@rsbuild/core";
import { pluginReact } from "@rsbuild/plugin-react";
import { tanstackRouter } from "@tanstack/router-plugin/rspack";

const __dirname = path.dirname(fileURLToPath(import.meta.url));

export default defineConfig(({ envMode }) => {
  const isProd = envMode === "production";
  const serverUrl = process.env.CMH_DEV_SERVER_URL || "http://localhost:3000";
  return {
    plugins: [pluginReact()],
    source: { entry: { index: "./src/main.tsx" } },
    resolve: {
      alias: {
        "@": path.resolve(__dirname, "../web/src"),
        "~": path.resolve(__dirname, "./src"),
      },
      modules: [path.resolve(__dirname, "./node_modules"), "node_modules"],
    },
    html: { template: "./index.html" },
    // Everything is mounted under /cmh/ by ext; assets must resolve there too.
    output: {
      assetPrefix: "/cmh/",
      distPath: { root: process.env.CMH_DIST_DIR || "../ext/console/dist" },
      minify: isProd,
      target: "web",
    },
    server: {
      host: "0.0.0.0",
      port: 3100,
      // Dev only: forward API calls to a running gateway so the session cookie
      // is first-party. Production is served by the gateway itself.
      proxy: { "/api": { target: serverUrl, changeOrigin: true } },
    },
    tools: {
      rspack: {
        resolve: { modules: [path.resolve(__dirname, "./node_modules"), "node_modules"] },
        plugins: [tanstackRouter({ target: "react", autoCodeSplitting: isProd })],
      },
    },
  };
});
