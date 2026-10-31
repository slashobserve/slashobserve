import { describe, expect, it } from "vitest"

import { sameJson } from "@/lib/json"
import type { DataPanel, PanelQuery, TextPanel } from "@/spec"

import {
  changeSummary,
  defaultSaveSummary,
  normalisePanel,
  panelChanges,
  queryChanged,
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

  it("reports a changed expression by its ref and ignores an identical one", () => {
    const expression = (math: string): DataPanel["queries"][number] => ({
      ref: "E",
      expression: { type: "math", math },
    })
    const base = panel({ queries: [{ query: "up" }, expression("$A")] })
    expect(
      panelChanges(
        base,
        panel({ queries: [{ query: "up" }, expression("$A")] })
      )
    ).toEqual([])
    expect(
      panelChanges(
        base,
        panel({ queries: [{ query: "up" }, expression("$A * 2")] })
      )
    ).toEqual([{ key: "queries.E.expression", label: "expression E" }])
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

  it("ignores surrounding whitespace in a legend and an empty legend", () => {
    const base = panel({ queries: [{ query: "up", legend: "x" }] })
    expect(
      labels(base, panel({ queries: [{ query: "up", legend: " x " }] }))
    ).toEqual([])
    expect(
      labels(panel(), panel({ queries: [{ query: "up", legend: "  " }] }))
    ).toEqual([])
    expect(
      labels(base, panel({ queries: [{ query: "up", legend: " " }] }))
    ).toEqual(["legend"])
  })

  it("names a valueless extra step as a threshold change", () => {
    const base = panel({
      thresholds: [{ color: "ok" }, { value: 80, color: "warning" }],
    })
    const draft = panel({
      thresholds: [
        { color: "ok" },
        { value: 80, color: "warning" },
        { color: "critical" },
      ],
    })
    expect(panelChanges(base, draft)).toEqual([
      { key: "thresholds", label: "thresholds" },
    ])
  })

  it("names only the base color when the first step changes color", () => {
    const base = panel({
      thresholds: [{ color: "ok" }, { value: 80, color: "warning" }],
    })
    const draft = panel({
      thresholds: [{ color: "critical" }, { value: 80, color: "warning" }],
    })
    expect(labels(base, draft)).toEqual(["base color"])
  })

  it("treats a first step that has a value like any other step", () => {
    const base = panel({ thresholds: [{ value: 10, color: "ok" }] })
    const draft = panel({ thresholds: [{ value: 20, color: "ok" }] })
    expect(labels(base, draft)).toEqual(["ok threshold"])
  })

  it("ignores an explicit ref that equals the default", () => {
    const base = panel({ queries: [{ query: "a" }, { query: "b" }] })
    const draft = panel({
      queries: [
        { ref: "A", query: "a" },
        { ref: "B", query: "b" },
      ],
    })
    expect(panelChanges(base, draft)).toEqual([])
  })

  it("reports queries that only swapped their refs", () => {
    const base = panel({
      queries: [
        { ref: "A", query: "up" },
        { ref: "B", query: "up" },
      ],
    })
    const draft = panel({
      queries: [
        { ref: "B", query: "up" },
        { ref: "A", query: "up" },
      ],
    })
    expect(panelChanges(base, draft)).toEqual([
      { key: "queries", label: "queries" },
    ])
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

  it("falls back to the key for a key named like an Object member", () => {
    const draft = { ...panel(), constructor: 1 } as unknown as DataPanel
    expect(panelChanges(panel(), draft)).toEqual([
      { key: "constructor", label: "constructor" },
    ])
    expect(
      panelChanges(panel(), panel({ display: { constructor: 1 } })).map(
        (change) => change.label
      )
    ).toEqual(["constructor"])
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

describe("queryChanged", () => {
  it("ignores the ref, which matching queries share anyway", () => {
    expect(queryChanged({ query: "a" }, { ref: "A", query: "a" })).toBe(false)
  })

  it("sees a change to any other field", () => {
    expect(queryChanged({ query: "a" }, { query: "b" })).toBe(true)
    expect(queryChanged({ query: "a" }, { query: "a", legend: "x" })).toBe(true)
    expect(queryChanged({ query: "a" }, { query: "a", hide: true })).toBe(true)
  })

  it("reads a legend the way a save does", () => {
    expect(queryChanged({ query: "a" }, { query: "a", legend: "  " })).toBe(
      false
    )
    expect(
      queryChanged({ query: "a", legend: "x" }, { query: "a", legend: " x " })
    ).toBe(false)
    expect(
      queryChanged({ query: "a", legend: "x" }, { query: "a", legend: "  " })
    ).toBe(true)
  })
})

describe("normalisePanel", () => {
  it("trims the title and every legend and drops an empty legend", () => {
    const result = normalisePanel(
      panel({
        title: "  CPU ",
        queries: [
          { query: " up ", legend: "  {{pod}} " },
          { ref: "B", query: "b", legend: "   " },
          { ref: "C", query: "c", legend: "" },
        ],
      })
    ) as DataPanel
    expect(result.title).toBe("CPU")
    expect(result.queries).toEqual([
      { query: " up ", legend: "{{pod}}" },
      { ref: "B", query: "b" },
      { ref: "C", query: "c" },
    ])
  })

  it("keeps a panel without a title without one", () => {
    const untitled: Partial<DataPanel> = panel()
    delete untitled.title
    expect("title" in normalisePanel(untitled as DataPanel)).toBe(false)
  })

  it("trims the title of a text panel and leaves its content", () => {
    const text: TextPanel = {
      id: "t",
      type: "text",
      title: " Notes ",
      gridPos,
      content: " keep ",
    }
    expect(normalisePanel(text)).toEqual({
      ...text,
      title: "Notes",
      content: " keep ",
    })
  })

  it("does not change its input", () => {
    const input = panel({
      title: " CPU ",
      queries: [{ query: "up", legend: " x " }],
    })
    normalisePanel(input)
    expect(input.title).toBe(" CPU ")
    expect(input.queries[0]).toMatchObject({ legend: " x " })
  })
})

describe("panelChanges against the JSON", () => {
  function random(seed: number) {
    let state = seed
    return () => {
      state = (state + 0x6d2b79f5) | 0
      let t = Math.imul(state ^ (state >>> 15), 1 | state)
      t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t
      return ((t ^ (t >>> 14)) >>> 0) / 4294967296
    }
  }

  const start = (): DataPanel =>
    panel({
      title: "CPU",
      unit: "percent",
      queries: [{ query: "a" }, { ref: "B", query: "b" }],
      thresholds: [
        { color: "ok" },
        { value: 50, color: "warning" },
        { value: 80, color: "#ff0000" },
      ],
      display: { legend: "bottom", tooltip: "all" },
    } as Partial<DataPanel>)

  const mutations: ((p: DataPanel) => void)[] = [
    (p) => (p.title = `${p.title} `),
    (p) => (p.title = ` ${p.title}`),
    (p) => (p.title = "Other"),
    (p) => (p.unit = "bytes"),
    (p) => delete p.unit,
    (p) => (p.type = "stat"),
    (p) => (p.datasource = "other"),
    (p) => (p.queries = p.queries.slice().reverse()),
    (p) => (p.queries[0] = { ...p.queries[0], ref: "A" }),
    (p) => (p.queries[1] = { query: "b" }),
    (p) => (p.queries[0] = { ...p.queries[0], legend: "x" }),
    (p) => (p.queries[0] = { ...p.queries[0], legend: " x " }),
    (p) => (p.queries[1] = { ...p.queries[1], legend: "  " }),
    (p) => (p.queries[1] = { ...p.queries[1], legend: "" }),
    (p) => p.queries.push({ ref: "C", query: "c" }),
    (p) => p.queries.length > 1 && p.queries.pop(),
    (p) => p.thresholds?.push({ color: "critical" }),
    (p) => p.thresholds?.push({ value: 90, color: "critical" }),
    (p) => p.thresholds?.reverse(),
    (p) => delete p.thresholds,
    (p) => (p.thresholds = []),
    (p) => (p.thresholds = [{ color: "info" }]),
    (p) => (p.thresholds = [{ value: 5, color: "info" }]),
    (p) => p.thresholds && (p.thresholds[1] = { value: 51, color: "warning" }),
    (p) => p.thresholds && (p.thresholds[2] = { value: 80, color: "#00ff00" }),
    (p) => (p.display = { ...p.display, legend: "right" }),
    (p) => delete p.display,
    (p) => (p.display = {}),
    (p) => (p.description = "d"),
    (p) => (p.decimals = 2),
  ]

  const normalized = (p: DataPanel) => ({
    ...p,
    title: p.title.trim(),
    queries: (p.queries as PanelQuery[]).map(({ legend, ...rest }, index) => ({
      ...rest,
      ...(legend?.trim() ? { legend: legend.trim() } : {}),
      ref: rest.ref ?? String.fromCharCode(65 + index),
    })),
  })
  function mutated(next: () => number, count: number): DataPanel {
    const result = start()
    for (let i = 0; i < count; i++) {
      mutations[Math.floor(next() * mutations.length)](result)
    }
    return result
  }

  it("names an empty display object added to a base without display", () => {
    const base = panel()
    expect(panelChanges(base, { ...base, display: {} })).toEqual([
      { key: "display", label: "display" },
    ])
  })

  it("is empty exactly when the panels are the same JSON apart from title trimming and default refs", () => {
    const next = random(156)
    const seen = {
      sameDespiteRawDifference: 0,
      different: 0,
      emptyDisplayOverAbsent: 0,
      emptyThresholdsOverAbsent: 0,
    }
    for (let round = 0; round < 20000; round++) {
      const base = mutated(next, Math.floor(next() * 4))
      const draft = structuredClone(base)
      const count = Math.floor(next() * 4)
      for (let i = 0; i < count; i++) {
        mutations[Math.floor(next() * mutations.length)](draft)
      }
      const same = sameJson(normalized(base), normalized(draft))
      if (same && !sameJson(base, draft)) seen.sameDespiteRawDifference++
      if (!same) seen.different++
      if (!("display" in base) && sameJson(draft.display, {}))
        seen.emptyDisplayOverAbsent++
      if (!("thresholds" in base) && sameJson(draft.thresholds, []))
        seen.emptyThresholdsOverAbsent++
      expect(
        panelChanges(base, draft).length === 0,
        JSON.stringify({ base, draft })
      ).toBe(same)
    }
    expect(seen.sameDespiteRawDifference).toBeGreaterThan(300)
    expect(seen.different).toBeGreaterThan(1000)
    expect(seen.emptyDisplayOverAbsent).toBeGreaterThan(20)
    expect(seen.emptyThresholdsOverAbsent).toBeGreaterThan(20)
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

  it("treats a title that only gained whitespace as unchanged", () => {
    const base = panel({ title: "CPU" })
    const draft = panel({ title: "CPU ", unit: "bytes" })
    const latest = panel({ title: "CPU usage" })
    expect(labels(base, draft)).toEqual(["unit"])
    const result = rebasePanel(base, draft, latest)
    expect(result.title).toBe("CPU usage")
    expect((result as DataPanel).unit).toBe("bytes")
  })

  it("keeps the latest queries when the draft only made refs explicit", () => {
    const base = panel({ queries: [{ query: "a" }] })
    const draft = panel({ queries: [{ ref: "A", query: "a" }], unit: "bytes" })
    const latest = panel({ queries: [{ query: "a2" }] })
    expect((rebasePanel(base, draft, latest) as DataPanel).queries).toEqual([
      { query: "a2" },
    ])
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
