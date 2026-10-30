import { createElement, Fragment } from "react"
import { renderToStaticMarkup } from "react-dom/server"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { renderHtml } from "./html"

beforeEach(() => {
  vi.stubGlobal("location", { origin: "https://observe.example.com" })
})

afterEach(() => {
  vi.unstubAllGlobals()
})

function render(html: string): string {
  return renderToStaticMarkup(createElement(Fragment, null, renderHtml(html)))
}

describe("renderHtml scripts and handlers", () => {
  it("drops a script element with its content and keeps the text around it", () => {
    const html = render("<script>alert(1)</script>ok")
    expect(html).not.toMatch(/<script/i)
    expect(html).not.toContain("alert(1)")
    expect(html).toContain("ok")
  })

  it.each(["style", "template", "noscript"])(
    "drops a %s element with its content",
    (tag) => {
      const html = render(`<${tag}>secret-content</${tag}>after`)
      expect(html).not.toContain(`<${tag}`)
      expect(html).not.toContain("secret-content")
      expect(html).toContain("after")
    }
  )

  it("drops event handler attributes", () => {
    const html = render('<div onclick="alert(1)" onmouseover=x>t</div>')
    expect(html).not.toMatch(/\bon\w+=/i)
    expect(html).toContain("t")
  })
})

describe("renderHtml images", () => {
  it("drops an event handler from an image", () => {
    const html = render("<img src=x onerror=alert(1)>")
    expect(html).not.toContain("onerror")
  })

  it("loads an https image through the server without a referrer", () => {
    const html = render('<img src="https://a.example/x.png" alt="logo">')
    expect(html).toContain(
      'src="/api/v1/assets/image?url=https%3A%2F%2Fa.example%2Fx.png"'
    )
    expect(html).toContain('referrerPolicy="no-referrer"')
    expect(html).toContain('alt="logo"')
  })

  it.each(["data:image/svg+xml;base64,PHN2Zz4=", "javascript:alert(1)"])(
    "shows only the alt text for the source %s",
    (src) => {
      const html = render(`<img src="${src}" alt="alt text">`)
      expect(html).not.toContain("<img")
      expect(html).toContain("alt text")
    }
  )

  it("keeps a numeric width and height", () => {
    const html = render('<img src="/a.png" alt="a" width="120" height="40">')
    expect(html).toContain("width:120px")
    expect(html).toContain("height:40px")
  })
})

describe("renderHtml links", () => {
  it("renders no javascript link", () => {
    const html = render('<a href="javascript:alert(1)">x</a>')
    expect(html).not.toContain("javascript:")
    expect(html).not.toContain("href")
    expect(html).toContain("x")
  })

  it("opens an external link in a new tab without opener", () => {
    const html = render('<a href="https://a.example">x</a>')
    expect(html).toContain('href="https://a.example"')
    expect(html).toContain('rel="noopener noreferrer"')
    expect(html).toContain('target="_blank"')
  })

  it("keeps a mailto link", () => {
    expect(render('<a href="mailto:a@b.example">x</a>')).toContain(
      'href="mailto:a@b.example"'
    )
  })
})

describe("renderHtml tags", () => {
  it.each([
    "script",
    "style",
    "iframe",
    "frame",
    "object",
    "embed",
    "form",
    "input",
    "button",
    "select",
    "textarea",
    "link",
    "meta",
    "base",
    "svg",
    "math",
    "video",
    "audio",
    "source",
    "track",
    "canvas",
    "template",
    "slot",
    "dialog",
    "portal",
  ])("never renders a %s element", (tag) => {
    const html = render(`<${tag} src="https://x.example">x</${tag}>`)
    expect(html).not.toMatch(new RegExp(`<${tag}\\b`, "i"))
  })

  it("drops a form with its input and button", () => {
    const html = render(
      '<form action="https://x"><input name="q"><button>go</button></form>'
    )
    expect(html).not.toMatch(/<(form|input|button)\b/i)
    expect(html).not.toContain("https://x")
  })

  it("keeps a disabled checkbox, as a task list writes it", () => {
    expect(render('<input type="checkbox" disabled>')).toContain("<input")
  })

  it("drops a checkbox that is not disabled", () => {
    expect(render('<input type="checkbox">')).not.toContain("<input")
  })

  it("drops svg and its xlink link", () => {
    const html = render(
      '<svg><a xlink:href="javascript:alert(1)">x</a></svg>after'
    )
    expect(html).not.toMatch(/<svg/i)
    expect(html).not.toContain("xlink")
    expect(html).not.toContain("javascript:")
    expect(html).toContain("after")
  })

  it("drops math", () => {
    expect(render("<math><mi>x</mi></math>")).not.toMatch(/<math/i)
  })

  it("drops an iframe", () => {
    const html = render('<iframe src="https://x.example"></iframe>')
    expect(html).not.toContain("iframe")
    expect(html).not.toContain("x.example")
  })

  it("keeps center, font, tables with spans and details", () => {
    expect(render('<center><font color="red">t</font></center>')).toMatch(
      /<center><font color="red">t<\/font><\/center>/
    )
    const table = render(
      '<table border="1"><tr><td colspan="2">c</td></tr></table>'
    )
    expect(table).toContain('border="1"')
    expect(table).toContain('colSpan="2"')
    expect(render("<details><summary>s</summary>d</details>")).toContain(
      "<details><summary>s</summary>d</details>"
    )
  })
})

describe("renderHtml attributes", () => {
  it("prefixes an id so it cannot shadow a page global", () => {
    expect(render('<div id="config">x</div>')).toContain(
      'id="user-content-config"'
    )
  })

  it.each([
    [
      '<img src="/a.png" srcset="https://x.example/a.png 2x" alt="a">',
      "srcset",
    ],
    ['<a href="https://x.example" ping="https://y.example">x</a>', "ping"],
    ['<td background="https://x.example/a.png">x</td>', "background"],
    ['<button formaction="https://x.example">x</button>', "formaction"],
  ])("drops the attribute in %s", (html, attribute) => {
    expect(render(html).toLowerCase()).not.toContain(attribute)
  })
})

describe("renderHtml styles", () => {
  it("keeps allowed declarations and drops the others", () => {
    expect(
      render('<div style="position:fixed;top:0;color:red">x</div>')
    ).toContain('style="color:red"')
  })

  it.each([
    "background:url(https://x.example/a.png)",
    "background-color:red;background-image:url(//x.example)",
    "color:var(--x)",
    "color:expression(alert(1))",
    "color:\\72 ed",
    "margin:attr(x)",
    "color:red /* c */",
    "display:contents",
    "z-index:99999",
    "transform:scale(100)",
    "pointer-events:none",
    "width:image-set(x 1x)",
    "color:env(x)",
  ])("drops the style %s", (style) => {
    const html = render(`<div style="${style}">x</div>`)
    expect(html).not.toContain("url(")
    expect(html).not.toContain("var(")
    expect(html).not.toContain("expression(")
    expect(html).not.toContain("z-index")
    expect(html).not.toContain("transform")
    expect(html).not.toContain("pointer-events")
    expect(html).not.toContain("display:contents")
    expect(html).not.toContain("attr(")
    expect(html).not.toContain("env(")
    expect(html).not.toContain("image-set(")
    expect(html).not.toContain("/*")
  })

  it("drops the attribute when no declaration is left", () => {
    expect(render('<div style="background:url(https://x)">x</div>')).toBe(
      "<div>x</div>"
    )
    expect(render('<div style="color:var(--x)">x</div>')).toBe("<div>x</div>")
  })

  it("matches properties in any case and keeps the allowed display values", () => {
    expect(render('<div style="COLOR:Red;Display:Flex">x</div>')).toContain(
      'style="color:Red;display:Flex"'
    )
  })

  it("keeps styles on elements the panel draws itself", () => {
    expect(render('<p style="text-align:center">x</p>')).toContain(
      'style="text-align:center"'
    )
  })
})
