import { mkdirSync, readFileSync, writeFileSync } from "node:fs"
import { dirname, join, resolve } from "node:path"
import { fileURLToPath } from "node:url"
import { build } from "vite"

const webRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..")
const out = join(webRoot, "dist-panel")
const target = resolve(webRoot, "../internal/server/mcp/panelapp")

export function inlineAssets(html, read) {
  const withStyles = html.replace(
    /<link rel="stylesheet"[^>]*href="([^"]+)"[^>]*>/g,
    (_tag, href) =>
      `<style>${read(href).replaceAll("</style", "<\\/style")}</style>`
  )
  const scripts = []
  const withoutScripts = withStyles.replace(
    /<script type="module"[^>]*src="([^"]+)"[^>]*><\/script>/g,
    (_tag, src) => {
      scripts.push(src)
      return ""
    }
  )
  if (scripts.length !== 1) {
    throw new Error(`expected one module script, found ${scripts.length}`)
  }
  const code = read(scripts[0]).replaceAll("</script", "<\\/script")
  return withoutScripts.replace(
    "</body>",
    () => `<script type="module">${code}</script></body>`
  )
}

async function main() {
  await build({
    configFile: join(webRoot, "vite.panel.config.ts"),
    logLevel: "warn",
  })
  const read = (href) =>
    readFileSync(join(out, href.replace(/^\//, "")), "utf8")
  const html = inlineAssets(read("/mcp-panel.html"), read)
  writeFileSync(join(out, "panel.html"), html)
  mkdirSync(target, { recursive: true })
  writeFileSync(join(target, "panel.html"), html)
}

if (import.meta.main) await main()
