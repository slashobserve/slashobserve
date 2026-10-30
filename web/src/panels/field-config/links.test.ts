import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import type { WireFrame } from "@/dashboards/queries"
import type { DashboardLink, DataPanel, Link } from "@/spec"

import {
  appendLinkParams,
  createFieldResolver,
  dashboardLinks,
  isDataLink,
  oneClickLink,
  panelLinks,
  resolveLink,
  type LinkScope,
} from "."

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

  it("keeps the All selection as its variable value in query parameters", () => {
    expect(href("/d?${pod:queryparam}", { vars: { pod: ["$__all"] } })).toBe(
      "/d?var-pod=%24__all"
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

  it("keeps the token when the row index is out of range", () => {
    const frame: WireFrame = {
      ref: "A",
      fields: [{ name: "route", type: "string", values: ["/a", "/b"] }],
    }
    for (const index of [2, -1]) {
      expect(
        href("/r?x=${__data.fields.route}", { row: { frame, index } })
      ).toBe("/r?x=${__data.fields.route}")
    }
  })

  it("reads a field of the row by its display name", () => {
    const frame: WireFrame = {
      ref: "A",
      fields: [
        { name: "Value", type: "number", values: [7, 9] },
        { name: "host", type: "string", values: ["a", "b"] },
      ],
      labels: { pod: "web" },
    }
    const fields = createFieldResolver({
      displayName: "Pod ${__field.labels.pod}",
    } as DataPanel)
    expect(
      href("/r?x=${__data.fields.Pod web}", {
        row: { frame, index: 1 },
        fields,
      })
    ).toBe("/r?x=9")
    expect(
      href("/r?x=${__data.fields.Pod web}", { row: { frame, index: 1 } })
    ).toBe("/r?x=${__data.fields.Pod web}")
  })

  it("prefers a field named like the token over another field's display name", () => {
    const frame: WireFrame = {
      ref: "A",
      fields: [
        { name: "Value", type: "number", values: [7, 9] },
        { name: "route", type: "string", values: ["/a", "/b"] },
      ],
    }
    const fields = createFieldResolver({ displayName: "route" } as DataPanel)
    expect(
      href("/r?x=${__data.fields.route}", { row: { frame, index: 1 }, fields })
    ).toBe("/r?x=%2Fb")
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

  it("formats variables as raw, csv, text or percentencode", () => {
    const vars = { pod: ["a b", "c/d"] }
    expect(href("/d?x=${pod:raw}", { vars })).toBe("/d?x=a b,c/d")
    expect(href("/d?x=${pod:csv}", { vars })).toBe("/d?x=a b,c/d")
    expect(href("/d?x=${pod:text}", { vars })).toBe("/d?x=a b + c/d")
    expect(href("/d?x=${pod:percentencode}", { vars })).toBe(
      "/d?x=a%20b%2Cc%2Fd"
    )
    expect(href("/d?x=${pod:nope}", { vars })).toBe("/d?x=${pod:nope}")
  })

  it("escapes the characters Grafana escapes when percent encoding", () => {
    const vars = { pod: ["foo()bar", "it's!*"] }
    expect(href("/d?x=${pod:percentencode}&y=1", { vars })).toBe(
      "/d?x=foo%28%29bar%2Cit%27s%21%2A&y=1"
    )
  })

  it("percent encodes a link made of one token", () => {
    expect(href("${pod:percentencode}", { vars: { pod: ["a b"] } })).toBe(
      "a%20b"
    )
  })

  it("formats a data field of the row", () => {
    const frame: WireFrame = {
      ref: "A",
      fields: [{ name: "route", type: "string", values: ["/a b", "/c d"] }],
    }
    const row = { frame, index: 1 }
    expect(href("/r?x=${__data.fields.route:raw}", { row })).toBe("/r?x=/c d")
    expect(href("/r?x=${__data.fields.route:percentencode}", { row })).toBe(
      "/r?x=%2Fc%20d"
    )
  })

  it("writes the time range as epoch milliseconds", () => {
    expect(
      href("/d?from=${__from}&to=${__to}", { fromMs: 1000, toMs: 2500 })
    ).toBe("/d?from=1000&to=2500")
    expect(href("/d?from=${__from}")).toBe("/d?from=${__from}")
  })

  it("reads a label of the series", () => {
    const labels = { pod: "web 1" }
    expect(href("/d?pod=${__field.labels.pod}", { labels })).toBe(
      "/d?pod=web%201"
    )
    expect(href("/d?x=${__field.labels.nope}", { labels })).toBe(
      "/d?x=${__field.labels.nope}"
    )
    expect(href("/d?x=${__field.labels.constructor}", { labels })).toBe(
      "/d?x=${__field.labels.constructor}"
    )
    expect(href("/d?x=${__field.labels.pod}")).toBe(
      "/d?x=${__field.labels.pod}"
    )
  })

  it("encodes substituted values", () => {
    expect(href("/d?q=${pod}", { vars: { pod: ["a b&c"] } })).toBe(
      "/d?q=a%20b%26c"
    )
  })

  it("keeps an unknown token as written", () => {
    expect(
      href("/d?x=${nope}&y=$other&z=${pod:bogus}", { vars: { pod: ["a"] } })
    ).toBe("/d?x=${nope}&y=$other&z=${pod:bogus}")
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

describe("a URL that is exactly one token", () => {
  const frame: WireFrame = {
    ref: "A",
    fields: [{ name: "url", type: "string", values: ["", "/x?a=b c&d=e"] }],
  }

  it("takes the value as it is, without encoding", () => {
    expect(
      href("${__value.raw}", { value: "https://example.org/a b?x=1&y=2" })
    ).toBe("https://example.org/a b?x=1&y=2")
    expect(href(" ${__data.fields.url} ", { row: { frame, index: 1 } })).toBe(
      "/x?a=b c&d=e"
    )
    expect(href("$link", { vars: { link: ["/d?x=1&y=2"] } })).toBe("/d?x=1&y=2")
  })

  it("still encodes a token inside a larger URL", () => {
    expect(
      href("https://example.org/?u=${__value.raw}", { value: "/a?b=c&d" })
    ).toBe("https://example.org/?u=%2Fa%3Fb%3Dc%26d")
  })

  it("drops a value with a scheme that is not allowed", () => {
    expect(
      href("${__value.raw}", { value: "javascript:alert(1)" })
    ).toBeUndefined()
    expect(
      href("${__data.fields.url}", {
        row: {
          frame: {
            ref: "A",
            fields: [
              { name: "url", type: "string", values: ["data:text/html,x"] },
            ],
          },
          index: 0,
        },
      })
    ).toBeUndefined()
    expect(
      href("$link", { vars: { link: [" JaVa\tScRiPt:alert(1)"] } })
    ).toBeUndefined()
  })
})

describe("link safety", () => {
  it("drops a javascript URL", () => {
    expect(href("javascript:alert(1)")).toBeUndefined()
    expect(href("JaVaScRiPt:alert(1)")).toBeUndefined()
    expect(href(" java\tscript:alert(1)")).toBeUndefined()
    expect(href("data:text/html,x")).toBeUndefined()
  })

  it("drops a URL that cannot be parsed instead of throwing", () => {
    expect(() => href("http://[")).not.toThrow()
    expect(href("http://[")).toBeUndefined()
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

  it("carries oneClick only when the link sets it", () => {
    expect(resolveLink({ title: "t", url: "/a" }, scope())).toEqual({
      title: "t",
      href: "/a",
      newTab: false,
    })
    expect(
      resolveLink({ title: "t", url: "/a", oneClick: true }, scope())
    ).toEqual({ title: "t", href: "/a", newTab: false, oneClick: true })
    expect(
      resolveLink({ title: "t", url: "/a", oneClick: false }, scope())?.oneClick
    ).toBeUndefined()
  })

  it("makes a link one click only when it is the single link and sets oneClick", () => {
    const plain = { title: "a", href: "/a", newTab: false }
    const quick = { ...plain, oneClick: true as const }
    expect(oneClickLink([quick])).toBe(quick)
    expect(oneClickLink([plain])).toBeUndefined()
    expect(oneClickLink([quick, plain])).toBeUndefined()
    expect(oneClickLink([])).toBeUndefined()
  })

  it("formats variables as pipe separated or JSON", () => {
    const vars = { pod: ["a", "b"] }
    expect(href("/d?x=${pod:pipe}", { vars })).toBe("/d?x=a|b")
    expect(href("/d?x=${pod:json}", { vars })).toBe('/d?x=["a","b"]')
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

  it("flags a one click link even when its address reads no data", () => {
    expect(isDataLink(link({ oneClick: true }))).toBe(true)
    expect(isDataLink(link({ oneClick: false }))).toBe(false)
  })

  it("keeps a one click link out of the panel header", () => {
    const panel = {
      links: [link({ oneClick: true }), link({ url: "/other" })],
    } as DataPanel
    expect(panelLinks(panel, scope()).map((shown) => shown.href)).toEqual([
      "/other",
    ])
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

  it("hides a link until the time range it uses is known", () => {
    const panel = {
      links: [
        { title: "Window", url: "/d?from=${__from}&to=$__to" },
        { title: "Plain", url: "/d" },
      ],
    } as DataPanel
    expect(panelLinks(panel, scope()).map((link) => link.title)).toEqual([
      "Plain",
    ])
    expect(
      panelLinks(panel, scope({ fromMs: 1000 })).map((link) => link.title)
    ).toEqual(["Plain"])
    expect(panelLinks(panel, scope({ fromMs: 1000, toMs: 2500 }))).toEqual([
      { title: "Window", href: "/d?from=1000&to=2500", newTab: false },
      { title: "Plain", href: "/d", newTab: false },
    ])
  })
})

describe("dashboardLinks", () => {
  const now = Date.parse("2026-10-21T15:04:05.678Z")
  const window: Link = { title: "Window", url: "/d?from=${__from}&to=${__to}" }

  it("fills the dashboard time range for a relative range", () => {
    expect(
      dashboardLinks([window], scope({ from: "now-6h", to: "now" }), now, {})
    ).toEqual([
      {
        title: "Window",
        href: `/d?from=${now - 6 * 3_600_000}&to=${now}`,
        newTab: false,
      },
    ])
  })

  it("rounds the end of the range up and the start down", () => {
    const [link] = dashboardLinks(
      [window],
      scope({ from: "now/d", to: "now/d" }),
      now,
      {}
    )
    expect(link.href).toBe(
      `/d?from=${Date.parse("2026-10-21T00:00:00Z")}&to=${Date.parse("2026-10-21T23:59:59.999Z")}`
    )
  })

  it("rounds in the dashboard's zone", () => {
    const [link] = dashboardLinks(
      [window],
      scope({ from: "now/d", to: "now/d" }),
      now,
      { zone: "Asia/Kolkata" }
    )
    expect(link.href).toBe(
      `/d?from=${Date.parse("2026-10-20T18:30:00Z")}&to=${Date.parse("2026-10-21T18:29:59.999Z")}`
    )
  })

  it("fills an absolute range as it is", () => {
    const [link] = dashboardLinks(
      [window],
      scope({ from: "1700000000000", to: "2026-01-02T03:04:05+02:00" }),
      now,
      {}
    )
    expect(link.href).toBe(
      `/d?from=1700000000000&to=${Date.parse("2026-01-02T01:04:05Z")}`
    )
  })

  it("still resolves variables and keeps links without a range token", () => {
    const links: Link[] = [
      window,
      { title: "Pod", url: "/d?pod=${pod}" },
      { title: "Range", url: "/d?${__url_time_range}" },
    ]
    expect(
      dashboardLinks(
        links,
        scope({ vars: { pod: ["a"] }, from: "now-1h", to: "now" }),
        now,
        {}
      ).map((link) => link.href)
    ).toEqual([
      `/d?from=${now - 3_600_000}&to=${now}`,
      "/d?pod=a",
      "/d?from=now-1h&to=now",
    ])
  })

  it("hides a link whose range cannot be resolved", () => {
    expect(
      dashboardLinks([window], scope({ from: "bogus", to: "now" }), now, {})
    ).toEqual([])
    expect(dashboardLinks([window], scope(), now, {})).toEqual([])
    expect(dashboardLinks(undefined, scope(), now, {})).toEqual([])
  })

  it("adds the range and the selections to the address on request", () => {
    const links: DashboardLink[] = [
      {
        title: "Both",
        url: "https://example.com/d",
        keepTime: true,
        includeVars: true,
      },
      { title: "Time", url: "/d?x=1", keepTime: true },
      { title: "Vars", url: "/d", includeVars: true },
      { title: "None", url: "/d" },
    ]
    expect(
      dashboardLinks(
        links,
        scope({ vars: { pod: ["a", "b c"] }, from: "now-1h", to: "now" }),
        now,
        {}
      ).map((link) => link.href)
    ).toEqual([
      "https://example.com/d?from=now-1h&to=now&var-pod=a&var-pod=b%20c",
      "/d?x=1&from=now-1h&to=now",
      "/d?var-pod=a&var-pod=b%20c",
      "/d",
    ])
  })
})

describe("appendLinkParams", () => {
  const range = { vars: { env: ["prod"] }, from: "now-6h", to: "now" }
  const both = { keepTime: true, includeVars: true }

  it.each([
    ["/d", "/d?from=now-6h&to=now&var-env=prod"],
    ["/d?", "/d?from=now-6h&to=now&var-env=prod"],
    ["/d?a=1&", "/d?a=1&from=now-6h&to=now&var-env=prod"],
    ["/d?a=1", "/d?a=1&from=now-6h&to=now&var-env=prod"],
    ["/d#top", "/d?from=now-6h&to=now&var-env=prod#top"],
    ["/d?a=1#top", "/d?a=1&from=now-6h&to=now&var-env=prod#top"],
    ["mailto:ops@example.com", "mailto:ops@example.com"],
  ])("puts the parameters of %s before the fragment", (href, want) => {
    expect(appendLinkParams(href, both, range)).toBe(want)
  })

  it("adds nothing without a flag or without a range to carry", () => {
    expect(appendLinkParams("/d", {}, range)).toBe("/d")
    expect(appendLinkParams("/d", { keepTime: true }, { vars: {} })).toBe("/d")
  })
})
