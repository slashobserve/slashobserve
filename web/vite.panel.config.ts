import { readdirSync, readFileSync } from "node:fs"
import { resolve } from "node:path"
import tailwindcss from "@tailwindcss/vite"
import react from "@vitejs/plugin-react"
import { defineConfig } from "vite"

const emptyFontCss = resolve(import.meta.dirname, "scripts/empty-font.css")
const panelsDir = resolve(import.meta.dirname, "src/panels")
const allManifestsGlob = '"./*/manifest.ts"'

function mcpAppManifests(): string[] {
  const found = readdirSync(panelsDir, { withFileTypes: true })
    .filter((entry) => entry.isDirectory())
    .map((entry) => entry.name)
    .filter((dir) => {
      try {
        return /^\s*mcpApp: true,/m.test(
          readFileSync(resolve(panelsDir, dir, "manifest.ts"), "utf8")
        )
      } catch {
        return false
      }
    })
    .sort()
  if (found.length === 0) {
    throw new Error("no panel manifest sets mcpApp: true")
  }
  return found.map((dir) => `"./${dir}/manifest.ts"`)
}

export default defineConfig({
  plugins: [
    {
      name: "panel-renderer-set",
      enforce: "pre",
      transform(code, id) {
        if (!id.endsWith("/src/panels/index.tsx")) return undefined
        if (!code.includes(allManifestsGlob)) {
          throw new Error(`${id} no longer globs ${allManifestsGlob}`)
        }
        return code.replace(
          allManifestsGlob,
          `[${mcpAppManifests().join(", ")}]`
        )
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
