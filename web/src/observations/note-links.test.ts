import { describe, expect, it } from "vitest"

import { stringifySearch } from "@/lib/search"
import type { Dashboard } from "@/spec"

import {
  dashboardPanelBody,
  logsLinkBody,
  maxNoteLinks,
  noteLinks,
  traceLinkBody,
} from "./note-links"

const origin = "https://obs.example.com"
const traceId = "4bf92f3577b34da6a3ce929d0e0e4736"
const dashboardId = "7f4a3b0c-1d2e-4f50-8a6b-9c0d1e2f3a4b"

describe("noteLinks", () => {
  it("finds a logs search with its query and range", () => {
    const query = '{app="checkout"} |= "timeout"'
    const url = `${origin}/logs/loki${stringifySearch({ q: query, from: "now-6h", to: "now" })}`
    const [link] = noteLinks(`Look at ${url} please`, origin)
    expect(link).toMatchObject({ kind: "logs", uid: "loki", query })
    expect(link?.kind === "logs" && link.search.from).toBe("now-6h")
  })

  it("finds a trace with its datasource and a dashboard panel", () => {
    const text = [
      `${origin}/t/${traceId}?ds=tempo`,
      `${origin}/d/${dashboardId}/checkout?panel=latency&from=now-1h&to=now`,
    ].join("\n")
    expect(noteLinks(text, origin)).toEqual([
      {
        kind: "trace",
        uid: "tempo",
        traceId,
        from: undefined,
        to: undefined,
      },
      {
        kind: "panel",
        dashboardId,
        panelId: "latency",
        from: "now-1h",
        to: "now",
      },
    ])
  })

  it("reads a viewed panel as the panel", () => {
    const [link] = noteLinks(
      `${origin}/d/${dashboardId}?viewPanel=errors`,
      origin
    )
    expect(link).toMatchObject({ kind: "panel", panelId: "errors" })
  })

  it("lets links from other origins stay text", () => {
    const text = `https://other.example.com/logs/loki?q=error ${origin.replace("https", "http")}/logs/loki?q=error`
    expect(noteLinks(text, origin)).toEqual([])
  })

  it("ignores this app's pages that make no block", () => {
    const text = [
      `${origin}/logs/loki`,
      `${origin}/t/not-a-trace?ds=tempo`,
      `${origin}/t/${traceId}`,
      `${origin}/d/${dashboardId}`,
      `${origin}/d/${dashboardId}/versions?panel=latency`,
      `${origin}/o/${dashboardId}`,
      `${origin}/logs/bad%20uid?q=error`,
    ].join("\n")
    expect(noteLinks(text, origin)).toEqual([])
  })

  it("drops punctuation after a link and reads one inside Markdown", () => {
    const url = `${origin}/logs/loki?q=error`
    expect(noteLinks(`See ${url}.`, origin)).toHaveLength(1)
    expect(noteLinks(`See [the logs](${url}), then more`, origin)).toHaveLength(
      1
    )
  })

  it("keeps a repeated link once", () => {
    const url = `${origin}/logs/loki?q=error`
    expect(noteLinks(`${url} and again ${url}`, origin)).toHaveLength(1)
  })

  it("keeps at most ten links", () => {
    const text = Array.from(
      { length: 15 },
      (_, index) => `${origin}/logs/loki?q=error${index}`
    ).join(" ")
    expect(noteLinks(text, origin)).toHaveLength(maxNoteLinks)
  })
})

describe("link blocks", () => {
  const now = Date.parse("2026-11-13T10:00:00Z")

  it("builds a logs panel over the linked range", () => {
    const [link] = noteLinks(
      `${origin}/logs/loki${stringifySearch({ q: '{app="a"}', from: "now-1h", to: "now" })}`,
      origin
    )
    if (link?.kind !== "logs") throw new Error("expected a logs link")
    const body = logsLinkBody(link, "loki", now)
    expect(body.panel).toMatchObject({
      type: "logs",
      datasource: "loki",
      queries: [{ query: '{app="a"}' }],
    })
    expect(body.panel).not.toHaveProperty("id")
    expect(body.from).toBe("2026-11-13T09:00:00.000Z")
    expect(body.to).toBe("2026-11-13T10:00:00.000Z")
  })

  it("builds a logs panel with no range when the link has none", () => {
    const [link] = noteLinks(`${origin}/logs/loki?q=error`, origin)
    if (link?.kind !== "logs") throw new Error("expected a logs link")
    const body = logsLinkBody(link, undefined, now)
    expect(body).not.toHaveProperty("from")
  })

  it("builds a traces panel that reads the trace id", () => {
    const [link] = noteLinks(`${origin}/t/${traceId}?ds=tempo`, origin)
    if (link?.kind !== "trace") throw new Error("expected a trace link")
    expect(traceLinkBody(link, "tempo", now).panel).toMatchObject({
      type: "traces",
      datasource: "tempo",
      queries: [{ query: traceId }],
    })
  })

  it("builds a trace by id query for a ClickHouse trace link", () => {
    const [link] = noteLinks(`${origin}/t/${traceId}?ds=ch`, origin)
    if (link?.kind !== "trace") throw new Error("expected a trace link")
    const panel = traceLinkBody(link, "clickhouse", now).panel
    expect(panel).toMatchObject({ type: "traces", datasource: "ch" })
    const [query] = panel.queries as { query: string; format: string }[]
    expect(query?.format).toBe("table")
    expect(query?.query).toMatch(/^SELECT /)
    expect(query?.query).toContain(
      `FROM otel_traces WHERE TraceId = '${traceId}'`
    )
  })

  const spec = {
    specVersion: "0",
    title: "Checkout",
    panels: [
      {
        id: "latency",
        type: "timeseries",
        title: "Latency",
        datasource: "prom",
        gridPos: { x: 0, y: 0, w: 12, h: 8 },
        queries: [{ query: "up" }],
      },
      {
        id: "notes",
        type: "text",
        gridPos: { x: 0, y: 8, w: 12, h: 4 },
        content: "hello",
      },
    ],
  } as unknown as Dashboard

  it("copies a data panel of the dashboard without its place", () => {
    const [link] = noteLinks(`${origin}/d/${dashboardId}?panel=latency`, origin)
    if (link?.kind !== "panel") throw new Error("expected a panel link")
    const outcome = dashboardPanelBody(link, spec, now)
    if (!("body" in outcome)) throw new Error("expected a block")
    expect(outcome.body.panel).toMatchObject({
      type: "timeseries",
      title: "Latency",
    })
    expect(outcome.body.panel).not.toHaveProperty("id")
    expect(outcome.body.panel).not.toHaveProperty("gridPos")
  })

  it("says why a missing panel or a text panel makes no block", () => {
    const missing = noteLinks(
      `${origin}/d/${dashboardId}?panel=gone`,
      origin
    )[0]
    const text = noteLinks(`${origin}/d/${dashboardId}?panel=notes`, origin)[0]
    if (missing?.kind !== "panel" || text?.kind !== "panel") {
      throw new Error("expected panel links")
    }
    expect(dashboardPanelBody(missing, spec, now)).toEqual({
      reason: "The panel is no longer on that dashboard.",
    })
    expect(dashboardPanelBody(text, spec, now)).toEqual({
      reason: "That panel holds no data to copy.",
    })
  })

  it("keeps the variables a linked panel uses, with the values in the link", () => {
    const withVariable = {
      ...spec,
      variables: [
        {
          name: "instance",
          type: "custom",
          multi: true,
          options: [
            { text: "a", value: "a" },
            { text: "b", value: "b" },
          ],
        },
        { name: "unused", type: "custom", options: [] },
      ],
      panels: [
        {
          id: "cpu",
          type: "timeseries",
          datasource: "prom",
          gridPos: { x: 0, y: 0, w: 12, h: 8 },
          queries: [{ query: 'up{instance=~"$instance"}' }],
        },
      ],
    } as unknown as Dashboard
    const link = noteLinks(
      `${origin}/d/${dashboardId}?panel=cpu&var-instance=a&var-instance=b`,
      origin
    )[0]
    if (link?.kind !== "panel") throw new Error("expected a panel link")
    const outcome = dashboardPanelBody(link, withVariable, now)
    if (!("body" in outcome)) throw new Error("expected a body")
    expect(outcome.body.variables).toHaveLength(1)
    expect(outcome.body.variables?.[0]).toMatchObject({
      name: "instance",
      default: ["a", "b"],
    })
  })

  it("treats built-in names the way the panel menu does", () => {
    const withBuiltIn = {
      ...spec,
      panels: [
        {
          id: "rate",
          type: "timeseries",
          datasource: "prom",
          gridPos: { x: 0, y: 0, w: 12, h: 8 },
          queries: [{ query: "rate(up[$__rate_interval])" }],
        },
      ],
    } as unknown as Dashboard
    const link = noteLinks(`${origin}/d/${dashboardId}?panel=rate`, origin)[0]
    if (link?.kind !== "panel") throw new Error("expected a panel link")
    const outcome = dashboardPanelBody(link, withBuiltIn, now)
    expect(outcome).toHaveProperty("body")
  })
})
