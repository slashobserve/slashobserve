import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import type { WireFrame } from "@/dashboards/queries"
import type { DataPanel, Link } from "@/spec"

import { isDataLink, panelLinks, resolveLink, type LinkScope } from "."

beforeEach(() => {
  vi.stubGlobal("location", { origin: "https://observe.example.com" })
})

afterEach(() => {
  vi.unstubAllGlobals()
})

const scope = (extra: Partial<LinkScope> = {}): LinkScope => ({
  vars: {},
  ...extra,
})

const href = (url: string, extra: Partial<LinkScope> = {}) =>
  resolveLink({ title: "t", url }, scope(extra))?.href

describe("link interpolation", () => {
  it("joins a multi-value variable with commas", () => {
    expect(href("/d?pods=${pod}", { vars: { pod: ["a", "b"] } })).toBe(
      "/d?pods=a%2Cb"
    )
    expect(href("/d?pod=$pod", { vars: { pod: ["a"] } })).toBe("/d?pod=a")
  })

  it("turns an All selection into the word All", () => {
    expect(href("/d?pod=${pod}", { vars: { pod: ["$__all"] } })).toBe(
      "/d?pod=All"
    )
  })

  it("writes a variable as query parameters", () => {
    expect(href("/d?${pod:queryparam}", { vars: { pod: ["a", "b c"] } })).toBe(
      "/d?var-pod=a&var-pod=b%20c"
    )
  })

  it("writes the time range", () => {
    expect(href("/d?${__url_time_range}", { from: "now-6h", to: "now" })).toBe(
      "/d?from=now-6h&to=now"
    )
    expect(href("/d?${__url_time_range}")).toBe("/d?${__url_time_range}")
  })

  it("reads a field of the row by name or in brackets", () => {
    const frame: WireFrame = {
      ref: "A",
      fields: [
        { name: "route", type: "string", values: ["/a", "/b"] },
        { name: "count", type: "number", values: [1, 2] },
      ],
    }
    const row = { frame, index: 1 }
    expect(href("/r?route=${__data.fields.route}", { row })).toBe(
      "/r?route=%2Fb"
    )
    expect(href('/r?n=${__data.fields["count"]}', { row })).toBe("/r?n=2")
    expect(href("/r?x=${__data.fields.missing}", { row })).toBe(
      "/r?x=${__data.fields.missing}"
    )
    expect(href("/r?x=${__data.fields.route}")).toBe(
      "/r?x=${__data.fields.route}"
    )
  })

  it("fills the value, series and field tokens", () => {
    const url =
      "/v?raw=${__value.raw}&text=${__value.text}&s=${__series.name}&f=${__field.name}"
    expect(
      href(url, {
        value: 0.5,
        valueText: "50%",
        seriesName: "web 1",
        fieldName: "Value",
      })
    ).toBe("/v?raw=0.5&text=50%25&s=web%201&f=Value")
  })

  it("encodes substituted values", () => {
    expect(href("/d?q=${pod}", { vars: { pod: ["a b&c"] } })).toBe(
      "/d?q=a%20b%26c"
    )
  })

  it("keeps an unknown token as written", () => {
    expect(
      href("/d?x=${nope}&y=$other&z=${pod:csv}", { vars: { pod: ["a"] } })
    ).toBe("/d?x=${nope}&y=$other&z=${pod:csv}")
  })

  it("does not look up variables on the prototype", () => {
    expect(href("/d?x=${constructor}")).toBe("/d?x=${constructor}")
  })

  it("does not interpolate substituted text again", () => {
    expect(href("/d?x=${a}", { vars: { a: ["${b}"], b: ["no"] } })).toBe(
      "/d?x=%24%7Bb%7D"
    )
  })

  it("interpolates the title as plain text", () => {
    const link = resolveLink(
      { title: "Pods ${pod}", url: "/d" },
      scope({ vars: { pod: ["a b", "c"] } })
    )
    expect(link?.title).toBe("Pods a b,c")
  })
})

describe("link safety", () => {
  it("drops a javascript URL", () => {
    expect(href("javascript:alert(1)")).toBeUndefined()
    expect(href("JaVaScRiPt:alert(1)")).toBeUndefined()
    expect(href(" java\tscript:alert(1)")).toBeUndefined()
    expect(href("data:text/html,x")).toBeUndefined()
  })

  it("drops a URL whose variable builds a javascript scheme", () => {
    expect(
      href("${scheme}:alert(1)", { vars: { scheme: ["javascript"] } })
    ).toBeUndefined()
  })

  it("keeps http, https, mailto and relative URLs", () => {
    expect(href("https://example.org/x")).toBe("https://example.org/x")
    expect(href("http://example.org/x")).toBe("http://example.org/x")
    expect(href("mailto:ops@example.org")).toBe("mailto:ops@example.org")
    expect(href("/dashboards/x")).toBe("/dashboards/x")
  })

  it("defaults newTab from the Spec defaults", () => {
    expect(resolveLink({ title: "t", url: "/a" }, scope())?.newTab).toBe(false)
    expect(
      resolveLink({ title: "t", url: "/a", newTab: true }, scope())?.newTab
    ).toBe(true)
  })
})

describe("data links", () => {
  const link = (extra: Partial<Link>): Link => ({
    title: "t",
    url: "/d",
    ...extra,
  })

  it("flags links that use data tokens in the url or the title", () => {
    expect(isDataLink(link({ url: "/d?r=${__data.fields.route}" }))).toBe(true)
    expect(isDataLink(link({ url: "/d?v=${__value.raw}" }))).toBe(true)
    expect(isDataLink(link({ title: "${__series.name}" }))).toBe(true)
    expect(isDataLink(link({ title: "${__field.name}" }))).toBe(true)
  })

  it("leaves variable and time range links as panel links", () => {
    expect(isDataLink(link({ url: "/d?${pod:queryparam}" }))).toBe(false)
    expect(isDataLink(link({ url: "/d?${__url_time_range}" }))).toBe(false)
    expect(isDataLink(link({}))).toBe(false)
  })

  it("lists only the panel links that are not data links", () => {
    const panel = {
      links: [
        { title: "Runbook", url: "https://example.org/rb?pod=${pod}" },
        { title: "Row", url: "/r?x=${__data.fields.route}" },
        { title: "Bad", url: "javascript:alert(1)" },
      ],
    } as DataPanel
    expect(panelLinks(panel, scope({ vars: { pod: ["a"] } }))).toEqual([
      { title: "Runbook", href: "https://example.org/rb?pod=a", newTab: false },
    ])
    expect(panelLinks({} as DataPanel, scope())).toEqual([])
  })
})
