import { createElement } from "react"
import { renderToStaticMarkup } from "react-dom/server"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import type { Selections } from "@/api/keys"

import TextPanel from "./text-panel"

beforeEach(() => {
  vi.stubGlobal("location", { origin: "https://observe.example.com" })
})

afterEach(() => {
  vi.unstubAllGlobals()
})

function render(content: string, vars: Selections = {}): string {
  const html = renderToStaticMarkup(
    createElement(TextPanel, {
      panel: {
        id: "tx",
        type: "text",
        gridPos: { x: 0, y: 0, w: 6, h: 4 },
        content,
      },
      width: 400,
      height: 200,
      vars,
    })
  )
  return html.replace(/^<div[^>]*>/, "").replace(/<\/div>$/, "")
}

describe("text panel markup", () => {
  it("renders markdown headings, tables and code", () => {
    const html = render("# Title\n\n| a | b |\n| - | - |\n| 1 | 2 |\n\n`x`")
    expect(html).toContain("<h1")
    expect(html).toContain("<table")
    expect(html).toContain("<td")
    expect(html).toContain("<code")
  })

  it("renders nothing inside the body for empty content", () => {
    expect(render("")).not.toMatch(/<(p|h\d|ul|table)\b/)
  })

  it("drops script elements written in the content", () => {
    const html = render("before\n\n<script>alert(1)</script>\n\nafter")
    expect(html).not.toMatch(/<script/i)
    expect(html).not.toContain("alert(1)")
    expect(html).toContain("before")
    expect(html).toContain("after")
  })

  it("drops raw img elements and their event handlers", () => {
    const html = render("a <img src=x onerror=alert(1)> b")
    expect(html).not.toMatch(/<img/i)
    expect(html).not.toContain("onerror")
  })

  it.each([
    "<iframe src='https://example.com'></iframe>",
    "<form action='/x'><input name='a'></form>",
    "<a href='javascript:alert(1)' onclick='alert(1)'>x</a>",
    "<div style='position:fixed;inset:0'>cover</div>",
    "<style>body{display:none}</style>",
    "<svg onload=alert(1)></svg>",
  ])("drops raw HTML %s", (raw) => {
    const html = render(`text\n\n${raw}\n\ntext`)
    expect(html).not.toMatch(/<(iframe|form|input|style|svg|div)\b/i)
    expect(html).not.toMatch(/\bon\w+=/i)
    expect(html).not.toContain("javascript:")
    expect(html).not.toContain("position:fixed")
  })
})

describe("text panel links", () => {
  it("opens an external link in a new tab without opener or referrer", () => {
    const html = render("[docs](https://example.com/docs)")
    expect(html).toContain('href="https://example.com/docs"')
    expect(html).toContain('target="_blank"')
    expect(html).toContain('rel="noopener noreferrer"')
    expect(html).toContain("<svg")
  })

  it("keeps a same-origin link in the tab", () => {
    const html = render("[dashboards](/dashboards)")
    expect(html).toContain('href="/dashboards"')
    expect(html).not.toContain("target=")
    expect(html).toContain('rel="noopener noreferrer"')
    expect(html).not.toContain("<svg")
  })

  it("keeps a mailto link without a new tab", () => {
    const html = render("[mail](mailto:oncall@example.com)")
    expect(html).toContain('href="mailto:oncall@example.com"')
    expect(html).not.toContain("target=")
  })

  it.each([
    "[bad](javascript:alert(1))",
    "[bad](JaVaScRiPt:alert(1))",
    "[bad](data:text/html;base64,PHNjcmlwdD4=)",
    "[bad](vbscript:msgbox(1))",
    "<javascript:alert(1)>",
    "[bad][ref]\n\n[ref]: javascript:alert(1)",
  ])("renders %j without a link", (markdown) => {
    const html = render(markdown)
    expect(html).not.toContain("<a")
    expect(html).not.toContain("href")
  })

  it("keeps the text of a link it refuses", () => {
    expect(render("[bad link](javascript:alert(1))")).toContain("bad link")
  })
})

describe("text panel images", () => {
  it("renders an https image lazily and without a referrer", () => {
    const html = render("![logo](https://cdn.example.com/a.png)")
    expect(html).toContain('src="https://cdn.example.com/a.png"')
    expect(html).toContain('loading="lazy"')
    expect(html).toContain('referrerPolicy="no-referrer"')
    expect(html).toContain('alt="logo"')
  })

  it("renders a same-origin image", () => {
    expect(render("![x](/static/a.png)")).toContain('src="/static/a.png"')
  })

  it.each([
    "![alt text](http://cdn.example.com/a.png)",
    "![alt text](data:image/svg+xml;base64,PHN2Zz4=)",
    "![alt text](javascript:alert(1))",
  ])("shows the alt text instead of %s", (markdown) => {
    const html = render(markdown)
    expect(html).not.toContain("<img")
    expect(html).toContain("alt text")
  })
})

describe("text panel variables", () => {
  it("substitutes variables before parsing", () => {
    const html = render("# Runbook for ${env}", { env: ["prod"] })
    expect(html).toContain("Runbook for prod")
    expect(html).toContain("<h1")
  })

  it("shows a value that looks like a link as plain text", () => {
    const html = render("Env: ${env}", { env: ["[x](https://evil.example)"] })
    expect(html).not.toContain("<a")
    expect(html.replaceAll("\u2060", "")).toContain("[x](https://evil.example)")
  })

  it("shows a value that looks like HTML as plain text", () => {
    const html = render("Env: ${env}", {
      env: ["<img src=x onerror=alert(1)>"],
    })
    expect(html).not.toContain("<img")
    expect(html).toContain("&lt;img src=x onerror=alert(1)&gt;")
  })

  it("does not let a value start a heading or list", () => {
    const html = render("Env: ${env}", { env: ["x\n\n# big\n\n- item"] })
    expect(html).not.toMatch(/<(h1|ul|li)\b/)
  })

  it("does not turn a bare web address in a value into a link", () => {
    const html = render("Env: ${env}", { env: ["https://evil.example"] })
    expect(html).not.toContain("<a")
  })
})
