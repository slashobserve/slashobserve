import { createElement } from "react"
import { renderToStaticMarkup } from "react-dom/server"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import type { Selections } from "@/api/keys"
import type { TextPanel as TextPanelSpec } from "@/spec"

import TextPanel from "./text-panel"

beforeEach(() => {
  vi.stubGlobal("location", { origin: "https://observe.example.com" })
})

afterEach(() => {
  vi.unstubAllGlobals()
})

type TextOptions = Pick<TextPanelSpec, "mode" | "code">

function renderWrapped(
  content: string,
  vars: Selections = {},
  options: TextOptions = {}
): string {
  return renderToStaticMarkup(
    createElement(TextPanel, {
      panel: {
        id: "tx",
        type: "text",
        gridPos: { x: 0, y: 0, w: 6, h: 4 },
        content,
        ...options,
      },
      width: 400,
      height: 200,
      vars,
      from: "now-1h",
      to: "now",
    })
  )
}

function render(
  content: string,
  vars: Selections = {},
  options: TextOptions = {}
): string {
  return renderWrapped(content, vars, options)
    .replace(/^<div[^>]*>/, "")
    .replace(/<\/div>$/, "")
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

  it("keeps a raw img element but removes its event handlers", () => {
    const html = render("a <img src=x onerror=alert(1)> b")
    expect(html).not.toContain("onerror")
  })

  it("loads a raw https img through the server", () => {
    const html = render('a <img src="https://cdn.example.com/a.png"> b')
    expect(html).toContain(
      `src="/api/v1/assets/image?url=${encodeURIComponent("https://cdn.example.com/a.png")}"`
    )
    expect(html).not.toContain('src="https://')
  })

  it("renders inline HTML in Markdown inside the sanitizer's rules", () => {
    const html = render(
      "<center><font color='red'>warn</font></center>\n\n<div style='position:fixed;color:red'>box</div>"
    )
    expect(html).toContain("<center>")
    expect(html).toContain('<font color="red">warn</font>')
    expect(html).toContain('<div style="color:red">box</div>')
  })

  it.each([
    "<iframe src='https://example.com'></iframe>",
    "<form action='/x'><input name='a'></form>",
    "<a href='javascript:alert(1)' onclick='alert(1)'>x</a>",
    "<div style='position:fixed;inset:0'>cover</div>",
    "<style>body{display:none}</style>",
    "<svg onload=alert(1)></svg>",
  ])("sanitizes raw HTML %s", (raw) => {
    const html = render(`text\n\n${raw}\n\ntext`)
    expect(html).not.toMatch(/<(iframe|form|input|style|svg)\b/i)
    expect(html).not.toMatch(/\bon\w+=/i)
    expect(html).not.toContain("javascript:")
    expect(html).not.toContain("position:fixed")
    expect(html).not.toContain("inset")
  })

  it("keeps the task list checkboxes of GitHub-flavoured Markdown", () => {
    expect(render("- [x] done\n- [ ] todo")).toContain(
      '<input type="checkbox" disabled="" checked=""/>'
    )
  })
})

describe("text panel modes", () => {
  it("defaults to Markdown", () => {
    const html = renderWrapped("# Title")
    expect(html).toContain('data-mode="markdown"')
    expect(html).toContain("<h1")
  })

  it("renders sanitized HTML in html mode, with Markdown left as text", () => {
    const html = renderWrapped(
      "# not a heading\n<p style='color:red' onclick='x()'>hi</p><script>alert(1)</script>",
      {},
      { mode: "html" }
    )
    expect(html).toContain('data-mode="html"')
    expect(html).toContain("# not a heading")
    expect(html).not.toContain("<h1")
    expect(html).toContain('<p style="color:red"')
    expect(html).not.toContain("onclick")
    expect(html).not.toContain("alert(1)")
  })

  it("loads an image in html mode through the server", () => {
    const html = render(
      '<img src="https://cdn.example.com/a.png" alt="logo">',
      {},
      { mode: "html" }
    )
    expect(html).toContain("/api/v1/assets/image?url=")
    expect(html).not.toContain('src="https://')
  })

  it("shows the content as code in code mode", () => {
    const html = renderWrapped(
      "# Title\n<script>alert(1)</script>",
      {},
      { mode: "code", code: { language: "markdown" } }
    )
    expect(html).toContain('data-mode="code"')
    expect(html).toContain('data-language="markdown"')
    expect(html).toContain("&lt;script&gt;alert(1)&lt;/script&gt;")
    expect(html).not.toContain("<h1")
    expect(html).not.toContain("<script")
  })

  it("numbers the lines of code, with the numbers hidden from readers", () => {
    const html = render(
      "a\nb",
      {},
      { mode: "code", code: { lineNumbers: true } }
    )
    expect(html).toContain('aria-hidden="true"')
    expect(html).toMatch(/>1<\/span>.*a<\/span>.*>2<\/span>.*b<\/span>/)
  })

  it("inserts a variable into code as written", () => {
    const html = render("x=${v}", { v: ["a&b"] }, { mode: "code" })
    expect(html).toContain("x=a&amp;b")
  })

  it("keeps whatever the HTML draws inside the panel box", () => {
    const html = renderWrapped("x", {}, { mode: "html" })
    expect(html).toContain("[contain:paint]")
    expect(html).toContain("isolate")
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
  it("loads an https image through the server, lazily and without a referrer", () => {
    const html = render("![logo](https://cdn.example.com/a.png)")
    expect(html).toContain(
      `src="/api/v1/assets/image?url=${encodeURIComponent("https://cdn.example.com/a.png")}"`
    )
    expect(html).not.toContain('src="https://')
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
