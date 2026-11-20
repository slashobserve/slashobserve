import { readdirSync, readFileSync, statSync, writeFileSync } from "node:fs"
import { dirname, join, resolve } from "node:path"
import { brotliCompressSync, constants } from "node:zlib"
import { fileURLToPath } from "node:url"

const assets = join(
  resolve(dirname(fileURLToPath(import.meta.url)), ".."),
  "dist",
  "assets"
)
const compressible = /\.(js|css|svg|json|html|txt|map)$/
const minBytes = 1024

for (const name of readdirSync(assets)) {
  const file = join(assets, name)
  if (!compressible.test(name) || statSync(file).size < minBytes) continue
  const source = readFileSync(file)
  const packed = brotliCompressSync(source, {
    params: {
      [constants.BROTLI_PARAM_QUALITY]: constants.BROTLI_MAX_QUALITY,
      [constants.BROTLI_PARAM_SIZE_HINT]: source.length,
    },
  })
  if (packed.length < source.length) writeFileSync(`${file}.br`, packed)
}
