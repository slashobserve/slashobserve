import { describe, expect, it } from "vitest"

import {
  assetGeojsonUrl,
  assetImageUrl,
  hostAllowed,
  remoteHost,
} from "./remote-assets"

const origin = "https://observe.example.com"

describe("assetImageUrl", () => {
  it("keeps a same-origin path, query and hash", () => {
    expect(assetImageUrl("/static/a.png?v=2#top", origin)).toBe(
      "/static/a.png?v=2#top"
    )
    expect(assetImageUrl("a.png", origin)).toBe("/a.png")
    expect(assetImageUrl("https://observe.example.com/b.png?x=1", origin)).toBe(
      "/b.png?x=1"
    )
  })

  it("keeps a same-origin http address on an http origin", () => {
    expect(
      assetImageUrl("http://localhost:5173/a.png", "http://localhost:5173")
    ).toBe("/a.png")
  })

  it("sends a remote https image through the server with the address encoded", () => {
    expect(
      assetImageUrl("https://cdn.example.com/a b.png?x=1&y=2#f", origin)
    ).toBe(
      "/api/v1/assets/image?url=" +
        encodeURIComponent("https://cdn.example.com/a%20b.png?x=1&y=2#f")
    )
  })

  it("treats a protocol-relative address as https", () => {
    expect(assetImageUrl("//cdn.example.com/x.png", origin)).toBe(
      "/api/v1/assets/image?url=" +
        encodeURIComponent("https://cdn.example.com/x.png")
    )
  })

  it("treats another port on the same host as remote", () => {
    expect(
      assetImageUrl("https://observe.example.com:8443/x.png", origin)
    ).toBe(
      "/api/v1/assets/image?url=" +
        encodeURIComponent("https://observe.example.com:8443/x.png")
    )
  })

  it.each([
    "http://cdn.example.com/a.png",
    "data:image/png;base64,AAAA",
    "javascript:alert(1)",
    "blob:https://example.com/x",
    "mailto:a@example.com",
    "http://[bad",
    "",
    "   ",
  ])("refuses %j", (raw) => {
    expect(assetImageUrl(raw, origin)).toBeNull()
  })
})

describe("assetGeojsonUrl", () => {
  it("sends a remote https file through the server", () => {
    expect(assetGeojsonUrl("https://raw.example.com/a.geojson", origin)).toBe(
      "/api/v1/assets/geojson?url=" +
        encodeURIComponent("https://raw.example.com/a.geojson")
    )
  })

  it.each([
    "/regions.geojson",
    "https://observe.example.com/regions.geojson",
    "http://raw.example.com/a.geojson",
    "data:application/json,{}",
    "javascript:alert(1)",
    "",
  ])("refuses %j", (raw) => {
    expect(assetGeojsonUrl(raw, origin)).toBeNull()
  })
})

describe("remoteHost", () => {
  it("returns the lower-cased host of a remote https address", () => {
    expect(remoteHost("https://CDN.Example.com/a.png", origin)).toBe(
      "cdn.example.com"
    )
    expect(remoteHost("//cdn.example.com/a.png", origin)).toBe(
      "cdn.example.com"
    )
  })

  it.each([
    "/a.png",
    "https://observe.example.com/a.png",
    "http://cdn.example.com/a.png",
    "data:image/png;base64,AAAA",
    "",
  ])("returns null for %j", (raw) => {
    expect(remoteHost(raw, origin)).toBeNull()
  })
})

describe("hostAllowed", () => {
  it("matches an exact pattern", () => {
    expect(hostAllowed(["cdn.example.com"], "cdn.example.com")).toBe(true)
    expect(hostAllowed(["cdn.example.com"], "img.example.com")).toBe(false)
    expect(hostAllowed(["example.com"], "cdn.example.com")).toBe(false)
  })

  it("matches a wildcard against subdomains only", () => {
    expect(hostAllowed(["*.example.com"], "cdn.example.com")).toBe(true)
    expect(hostAllowed(["*.example.com"], "a.b.example.com")).toBe(true)
    expect(hostAllowed(["*.example.com"], "example.com")).toBe(false)
    expect(hostAllowed(["*.example.com"], "badexample.com")).toBe(false)
  })

  it("ignores case on both sides", () => {
    expect(hostAllowed(["*.Example.COM"], "CDN.example.com")).toBe(true)
    expect(hostAllowed(["CDN.example.com"], "cdn.EXAMPLE.com")).toBe(true)
  })

  it("allows nothing for an empty list", () => {
    expect(hostAllowed([], "cdn.example.com")).toBe(false)
  })
})
