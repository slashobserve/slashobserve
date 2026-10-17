import { cpSync, existsSync, mkdirSync, readdirSync, rmSync } from "node:fs"
import { dirname, join, resolve } from "node:path"
import { fileURLToPath } from "node:url"

const webRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..")
const source = join(webRoot, "dist")
const target = resolve(webRoot, "../internal/server/webdist")
const keep = new Set([".gitkeep"])

if (!existsSync(source)) {
  throw new Error(`missing Vite output: ${source}`)
}

mkdirSync(target, { recursive: true })
for (const entry of readdirSync(target)) {
  if (!keep.has(entry)) {
    rmSync(join(target, entry), { recursive: true, force: true })
  }
}
cpSync(source, target, { recursive: true })
