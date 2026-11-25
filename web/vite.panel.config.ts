import { resolve } from "node:path"
import tailwindcss from "@tailwindcss/vite"
import react from "@vitejs/plugin-react"
import { defineConfig } from "vite"

import renderers from "./chart-types.json" with { type: "json" }

const emptyFontCss = resolve(import.meta.dirname, "scripts/empty-font.css")

export default defineConfig({
  plugins: [
    {
      name: "panel-renderer-set",
      enforce: "pre",
      transform(code, id) {
        if (!id.endsWith("/src/panels/index.tsx")) return undefined
        const globs = renderers.map((type) => `"./${type}/manifest.ts"`)
        return code.replace('"./*/manifest.ts"', `[${globs.join(", ")}]`)
      },
    },
    react(),
    tailwindcss(),
  ],
  resolve: {
    alias: [
      { find: "@", replacement: resolve(import.meta.dirname, "./src") },
      {
        find: /^@fontsource-variable\/geist(-mono)?$/,
        replacement: emptyFontCss,
      },
    ],
  },
  publicDir: false,
  build: {
    outDir: "dist-panel",
    emptyOutDir: true,
    cssCodeSplit: false,
    assetsInlineLimit: Number.MAX_SAFE_INTEGER,
    rolldownOptions: {
      input: resolve(import.meta.dirname, "mcp-panel.html"),
      output: { codeSplitting: false },
    },
  },
})
