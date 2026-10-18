import { writeFileSync } from "node:fs"
import { dirname, join, resolve } from "node:path"
import { fileURLToPath } from "node:url"

import { chromium } from "@playwright/test"

const publicDir = resolve(dirname(fileURLToPath(import.meta.url)), "../public")

const ink = "#0a0a0a"
const paper = "#ffffff"
const inkDark = "#fafafa"
const paperDark = "#111113"
const slash = "M14.6 5.6 9.4 18.4"
const radius = "5.8"

const faviconSvg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><style>rect{fill:${ink}}path{stroke:${paper}}@media (prefers-color-scheme:dark){rect{fill:${inkDark}}path{stroke:${paperDark}}}</style><rect width="24" height="24" rx="${radius}"/><path d="${slash}" fill="none" stroke-width="2.6" stroke-linecap="round"/></svg>`

function iconSvg(size, rounded) {
  const corner = rounded ? ` rx="${radius}"` : ""
  return `<svg xmlns="http://www.w3.org/2000/svg" width="${size}" height="${size}" viewBox="0 0 24 24"><rect width="24" height="24"${corner} fill="${ink}"/><path d="${slash}" fill="none" stroke="${paper}" stroke-width="2.6" stroke-linecap="round"/></svg>`
}

const pngs = [
  { file: "favicon-16.png", size: 16, rounded: true },
  { file: "favicon-32.png", size: 32, rounded: true },
  { file: "apple-touch-icon.png", size: 180, rounded: false },
]

const written = [join(publicDir, "favicon.svg")]
writeFileSync(written[0], faviconSvg + "\n")

const browser = await chromium.launch()
try {
  for (const { file, size, rounded } of pngs) {
    const context = await browser.newContext({
      viewport: { width: size, height: size },
      deviceScaleFactor: 1,
    })
    const page = await context.newPage()
    await page.setContent(
      `<html><body style="margin:0;background:transparent">${iconSvg(size, rounded)}</body></html>`
    )
    const path = join(publicDir, file)
    await page.screenshot({
      path,
      omitBackground: true,
      clip: { x: 0, y: 0, width: size, height: size },
    })
    await context.close()
    written.push(path)
  }
} finally {
  await browser.close()
}

for (const path of written) {
  console.log(path)
}
