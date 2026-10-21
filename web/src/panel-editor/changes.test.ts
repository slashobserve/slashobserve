import { describe, expect, it } from "vitest"

import type { DataPanel, TextPanel } from "@/spec"

import {
  changeSummary,
  defaultSaveSummary,
  panelChanges,
  rebasePanel,
  type Change,
} from "./changes"

const gridPos = { x: 0, y: 0, w: 6, h: 4 }

function panel(overrides: Partial<DataPanel> = {}): DataPanel {
  return {
    id: "p1",
    type: "timeseries",
    title: "Requests",
    gridPos,
    datasource: "prom",
    queries: [{ query: "up" }],
    ...overrides,
  }
}

function labels(base: DataPanel, draft: DataPanel): string[] {
  return panelChanges(base, draft).map((change) => change.label)
}

function changesOf(count: number): Change[] {
  return Array.from({ length: count }, (_, i) => ({
    key: `k${i}`,
    label: `label ${i}`,
  }))
}

describe("panelChanges", () => {
  it("reports nothing for an identical panel, whatever the key order", () => {
    const base = panel({ unit: "percent" })
    expect(panelChanges(base, structuredClone(base))).toEqual([])
    expect(
      panelChanges(base, {
        queries: [{ query: "up" }],
        unit: "percent",
        datasource: "prom",
        gridPos,
        title: "Requests",
        type: "timeseries",
        id: "p1",
      })
    ).toEqual([])
  })

  it("names a single query without its ref", () => {
    const base = panel()
    const draft = panel({
      queries: [{ query: "down", legend: "{{pod}}", datasource: "other" }],
    })
    expect(panelChanges(base, draft)).toEqual([
      { key: "queries.A.query", label: "query" },
      { key: "queries.A.legend", label: "legend" },
      { key: "queries.A.datasource", label: "query datasource" },
    ])
  })

  it("names several queries with their ref", () => {
    const base = panel({ queries: [{ query: "a" }, { query: "b" }] })
    const draft = panel({
      queries: [
        { query: "a", legend: "x" },
        { query: "b2", datasource: "other", instant: true },
      ],
    })
    expect(labels(base, draft)).toEqual([
      "legend A",
      "query B",
      "query B datasource",
      "query B options",
    ])
  })

  it("names an option change of a single query", () => {
    const draft = panel({ queries: [{ query: "up", interval: "30s" }] })
    expect(labels(panel(), draft)).toEqual(["query options"])
  })

  it("reports added and removed queries by ref", () => {
    const base = panel({ queries: [{ query: "a" }, { query: "b" }] })
    const draft = panel({ queries: [{ query: "a" }, { ref: "C", query: "c" }] })
    expect(panelChanges(base, draft)).toEqual([
      { key: "queries.C.new", label: "new query C" },
      { key: "queries.B.removed", label: "removed query B" },
    ])
  })

  it("matches a query by its ref, not its position", () => {
    const base = panel({ queries: [{ query: "a" }, { query: "b" }] })
    const draft = panel({
      queries: [{ ref: "B", query: "b" }],
    })
    expect(labels(base, draft)).toEqual(["removed query A"])
  })

  it("reports datasource, title, type, unit and content in order", () => {
    const draft = panel({
      datasource: "ch",
      title: "Other",
      type: "stat",
      unit: "bytes",
    })
    expect(labels(panel(), draft)).toEqual([
      "datasource",
      "title",
      "panel type",
      "unit",
    ])
    const text: TextPanel = { id: "t", type: "text", gridPos, content: "a" }
    expect(
      panelChanges(text, { ...text, content: "b" }).map(
        (change) => change.label
      )
    ).toEqual(["content"])
  })

  it("ignores surrounding whitespace in the title", () => {
    expect(labels(panel(), panel({ title: "  Requests " }))).toEqual([])
  })

  it("reports the canvas case as two changes", () => {
    const base = panel({
      thresholds: [{ color: "ok" }, { value: 80, color: "warning" }],
    })
    const draft = panel({
      queries: [{ query: "rate(up[5m])" }],
      thresholds: [{ color: "ok" }, { value: 90, color: "warning" }],
    })
    expect(changeSummary(panelChanges(base, draft))).toBe(
      "2 changes: query, warning threshold"
    )
  })

  it("names semantic thresholds that are added, removed or moved", () => {
    const base = panel({
      thresholds: [
        { color: "ok" },
        { value: 50, color: "info" },
        { value: 80, color: "warning" },
      ],
    })
    const draft = panel({
      thresholds: [
        { color: "ok" },
        { value: 60, color: "info" },
        { value: 95, color: "critical" },
      ],
    })
    expect(labels(base, draft)).toEqual([
      "info threshold",
      "warning threshold",
      "critical threshold",
    ])
  })

  it("names hex steps as one threshold and the first step as base color", () => {
    const base = panel({
      thresholds: [{ color: "#112233" }, { value: 1, color: "#aabbcc" }],
    })
    const draft = panel({
      thresholds: [{ color: "#445566" }, { value: 2, color: "#aabbcc" }],
    })
    expect(panelChanges(base, draft)).toEqual([
      { key: "thresholds.hex", label: "threshold" },
      { key: "thresholds.base", label: "base color" },
    ])
  })

  it("does not report an empty threshold list as a change", () => {
    expect(labels(panel(), panel({ thresholds: undefined }))).toEqual([])
  })

  it("labels each display key", () => {
    const base = panel({ display: { tooltip: "all", legend: "list" } })
    const draft = panel({
      display: { tooltip: "single", legendValues: ["mean"] },
    })
    expect(panelChanges(base, draft)).toEqual([
      { key: "display.legend", label: "legend" },
      { key: "display.legendValues", label: "legend values" },
      { key: "display.tooltip", label: "tooltip" },
    ])
  })

  it("labels every display key of the schema", () => {
    const keys = {
      reduce: "calculation",
      colorMode: "color mode",
      graph: "graph",
      textMode: "text mode",
      orientation: "orientation",
      thresholdMarkers: "threshold markers",
      header: "table header",
      footer: "table footer",
      pieType: "pie type",
      calculate: "heatmap calculation",
      yScale: "y scale",
      bucketSize: "bucket size",
      bucketCount: "bucket count",
      mergeValues: "merged values",
      showValues: "value labels",
      wrap: "line wrap",
      showTime: "time column",
      order: "order",
    }
    for (const [key, label] of Object.entries(keys)) {
      expect(labels(panel(), panel({ display: { [key]: 1 } }))).toEqual([label])
    }
  })

  it("labels other top-level keys in sorted order and falls back to the key", () => {
    const draft = {
      ...panel(),
      timeShift: "1h",
      description: "d",
      decimals: 2,
      futureKey: true,
    } as DataPanel
    expect(labels(panel(), draft)).toEqual([
      "decimals",
      "description",
      "futureKey",
      "time shift",
    ])
  })
})

describe("changeSummary", () => {
  it("says there are no changes", () => {
    expect(changeSummary([])).toBe("No changes")
  })

  it("uses the singular for one change", () => {
    expect(changeSummary(changesOf(1))).toBe("1 change: label 0")
  })

  it("lists three labels and counts the rest", () => {
    expect(changeSummary(changesOf(3))).toBe(
      "3 changes: label 0, label 1, label 2"
    )
    expect(changeSummary(changesOf(5))).toBe(
      "5 changes: label 0, label 1, label 2, and 2 more"
    )
  })
})

describe("defaultSaveSummary", () => {
  it("joins up to three labels and trims the title", () => {
    expect(defaultSaveSummary(changesOf(2), " CPU ")).toBe(
      'Changed label 0 and label 1 in "CPU"'
    )
    expect(defaultSaveSummary(changesOf(4), "CPU")).toBe(
      'Changed label 0, label 1 and label 2 and more in "CPU"'
    )
  })

  it("cuts at 500 code points without splitting a character", () => {
    const title = "😀".repeat(600)
    const summary = defaultSaveSummary(changesOf(1), title)
    expect(Array.from(summary)).toHaveLength(500)
    expect(summary.startsWith('Changed label 0 in "😀')).toBe(true)
  })
})

describe("rebasePanel", () => {
  it("keeps a teammate's change and applies the user's own", () => {
    const base = panel({ unit: "percent" })
    const draft = panel({ unit: "percent", title: "Mine" })
    const latest = panel({ unit: "bytes" })
    const result = rebasePanel(base, draft, latest)
    expect(result.title).toBe("Mine")
    expect((result as DataPanel).unit).toBe("bytes")
  })

  it("removes a key the user removed", () => {
    const base = panel({ unit: "percent" })
    const draft = panel()
    const latest = panel({ unit: "percent", title: "Theirs" })
    const result = rebasePanel(base, draft, latest)
    expect("unit" in result).toBe(false)
    expect(result.title).toBe("Theirs")
  })

  it("does not change the inputs", () => {
    const base = panel()
    const draft = panel({ title: "Mine" })
    const latest = panel({ unit: "bytes" })
    const result = rebasePanel(base, draft, latest)
    expect(result).not.toBe(latest)
    expect(latest.title).toBe("Requests")
    expect(base.title).toBe("Requests")
  })
})
