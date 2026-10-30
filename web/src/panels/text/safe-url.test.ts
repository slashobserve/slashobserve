import { describe, expect, it } from "vitest"

import { isExternal, safeUrl } from "./safe-url"

const origin = "https://observe.example.com"

describe("safeUrl", () => {
  it.each([
    "https://example.com/docs",
    "http://example.com",
    "mailto:oncall@example.com",
    "/dashboards",
    "../versions?x=1",
    "#section",
    "https://observe.example.com/dashboards",
  ])("keeps %s", (url) => {
    expect(safeUrl(url, origin)).toBe(url)
  })

  it.each([
    "javascript:alert(1)",
    "JaVaScRiPt:alert(1)",
    "  javascript:alert(1)",
    "java\tscript:alert(1)",
    "java\nscript:alert(1)",
    "data:text/html,<script>alert(1)</script>",
    "vbscript:msgbox(1)",
    "file:///etc/passwd",
    "blob:https://example.com/x",
    "http://[bad",
  ])("drops %j", (url) => {
    expect(safeUrl(url, origin)).toBe("")
  })

  it("keeps a protocol-relative URL", () => {
    expect(safeUrl("//other.example/x", origin)).toBe("//other.example/x")
  })
})

describe("isExternal", () => {
  it("treats another host as external", () => {
    expect(isExternal("https://example.com/docs", origin)).toBe(true)
  })

  it("resolves a protocol-relative URL to the page's protocol", () => {
    expect(isExternal("//other.example/x", origin)).toBe(true)
    expect(isExternal("//observe.example.com/x", origin)).toBe(false)
  })

  it("treats another scheme or port on the same host as external", () => {
    expect(isExternal("http://observe.example.com/x", origin)).toBe(true)
    expect(isExternal("https://observe.example.com:8443/x", origin)).toBe(true)
  })

  it("does not treat relative paths or the app's own origin as external", () => {
    expect(isExternal("/dashboards", origin)).toBe(false)
    expect(isExternal("https://observe.example.com/a", origin)).toBe(false)
  })

  it("does not treat mailto as external", () => {
    expect(isExternal("mailto:oncall@example.com", origin)).toBe(false)
  })
})
