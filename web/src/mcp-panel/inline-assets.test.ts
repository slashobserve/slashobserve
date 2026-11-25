import { describe, expect, it } from "vitest"

import { inlineAssets } from "../../scripts/build-panel-app.mjs"

const files: Record<string, string> = {
  "/assets/a.css": "body{color:red}/* </style> */",
  "/assets/a.js": 'const x = "</script>"; export {}',
}
const read = (href: string) => files[href]

const page =
  '<!doctype html><html><head><link rel="stylesheet" crossorigin href="/assets/a.css"><script type="module" crossorigin src="/assets/a.js"></script></head><body><div id="root"></div></body></html>'

describe("inlineAssets", () => {
  it("leaves no reference to a file and keeps the code runnable", () => {
    const html = inlineAssets(page, read)
    expect(html).not.toContain("/assets/")
    expect(html).toContain("<style>body{color:red}")
    expect(html.match(/<\/script>/g)).toHaveLength(1)
    expect(html.match(/<\/style>/g)).toHaveLength(1)
    expect(html).toContain('"<\\/script>"')
    expect(html.indexOf("<script")).toBeGreaterThan(html.indexOf("<div"))
  })

  it("refuses a page with more than one module script", () => {
    const two = page.replace(
      "</head>",
      '<script type="module" src="/assets/a.js"></script></head>'
    )
    expect(() => inlineAssets(two, read)).toThrow("found 2")
  })
})
