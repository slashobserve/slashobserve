import { describe, expect, it } from "vitest"

import type { DataPanel, PanelQuery, TextPanel } from "@/spec"

import {
  displayKeysFor,
  draftReducer,
  isEditablePanel,
  nextRef,
  sameJson,
  startDraft,
  stableJson,
  type DraftAction,
  type EditorDraft,
} from "./draft"

const gridPos = { x: 0, y: 0, w: 6, h: 4 }

function dataPanel(overrides: Partial<DataPanel> = {}): DataPanel {
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

const textPanel: TextPanel = {
  id: "t1",
  type: "text",
  gridPos,
  content: "hello",
}

function deepFreeze<T>(value: T): T {
  if (typeof value === "object" && value !== null) {
    for (const item of Object.values(value)) deepFreeze(item)
    Object.freeze(value)
  }
  return value
}

function apply(panel: DataPanel | TextPanel, ...actions: DraftAction[]) {
  return actions.reduce(
    (state, action) => draftReducer(state, action),
    startDraft(3, panel)
  )
}

function queriesOf(state: EditorDraft): PanelQuery[] {
  return (state.panel as DataPanel).queries
}

describe("stableJson", () => {
  it("sorts object keys at every depth and keeps array order", () => {
    expect(stableJson({ b: 1, a: { d: [2, 1], c: 3 } })).toBe(
      '{"a":{"c":3,"d":[2,1]},"b":1}'
    )
  })

  it("makes key order irrelevant to sameJson", () => {
    expect(sameJson({ a: 1, b: 2 }, { b: 2, a: 1 })).toBe(true)
    expect(sameJson([1, 2], [2, 1])).toBe(false)
  })
})

describe("startDraft", () => {
  it("clones the base into the draft", () => {
    const base = dataPanel()
    const state = startDraft(7, base)
    expect(state.baseVersion).toBe(7)
    expect(state.base).toBe(base)
    expect(state.panel).toEqual(base)
    expect(state.panel).not.toBe(base)
  })
})

describe("isEditablePanel", () => {
  it("accepts data and text panels only", () => {
    expect(isEditablePanel(dataPanel())).toBe(true)
    expect(isEditablePanel(textPanel)).toBe(true)
    expect(isEditablePanel({ id: "r", type: "row", title: "R", gridPos })).toBe(
      false
    )
    expect(
      isEditablePanel({
        id: "x",
        type: "placeholder",
        gridPos,
        reason: "unsupported",
      })
    ).toBe(false)
  })
})

describe("query actions", () => {
  it("sets the query text", () => {
    const state = apply(dataPanel(), { type: "setQuery", index: 0, query: "x" })
    expect(queriesOf(state)[0].query).toBe("x")
  })

  it("sets a legend and removes it when empty", () => {
    const withLegend = apply(dataPanel(), {
      type: "setLegend",
      index: 0,
      legend: "{{pod}}",
    })
    expect(queriesOf(withLegend)[0].legend).toBe("{{pod}}")
    const cleared = draftReducer(withLegend, {
      type: "setLegend",
      index: 0,
      legend: "",
    })
    expect("legend" in queriesOf(cleared)[0]).toBe(false)
  })

  it("sets a query datasource and removes it with null", () => {
    const set = apply(dataPanel(), {
      type: "setQueryDatasource",
      index: 0,
      uid: "other",
    })
    expect(queriesOf(set)[0].datasource).toBe("other")
    const cleared = draftReducer(set, {
      type: "setQueryDatasource",
      index: 0,
      uid: null,
    })
    expect("datasource" in queriesOf(cleared)[0]).toBe(false)
  })

  it("leaves the state alone for an index out of range or a text panel", () => {
    const data = startDraft(1, dataPanel())
    expect(draftReducer(data, { type: "setQuery", index: 4, query: "x" })).toBe(
      data
    )
    expect(
      draftReducer(data, { type: "setLegend", index: -1, legend: "x" })
    ).toBe(data)
    const text = startDraft(1, textPanel)
    expect(draftReducer(text, { type: "setQuery", index: 0, query: "x" })).toBe(
      text
    )
    expect(draftReducer(text, { type: "addQuery" })).toBe(text)
    expect(draftReducer(text, { type: "removeQuery", index: 0 })).toBe(text)
  })
})

describe("setPanelDatasource", () => {
  it("changes the panel datasource", () => {
    const state = apply(dataPanel(), { type: "setPanelDatasource", uid: "ch" })
    expect((state.panel as DataPanel).datasource).toBe("ch")
  })

  it("is ignored on a text panel", () => {
    const text = startDraft(1, textPanel)
    expect(draftReducer(text, { type: "setPanelDatasource", uid: "ch" })).toBe(
      text
    )
  })
})

describe("addQuery", () => {
  it("appends an empty query with the default ref omitted", () => {
    const state = apply(dataPanel(), { type: "addQuery" })
    expect(queriesOf(state)).toEqual([{ query: "up" }, { query: "" }])
  })

  it("gives the query a free ref when its position default is taken", () => {
    const state = apply(
      dataPanel({ queries: [{ query: "a" }, { ref: "C", query: "c" }] }),
      { type: "addQuery" }
    )
    expect(queriesOf(state)[2]).toEqual({ ref: "B", query: "" })
  })

  it("stops at 26 queries", () => {
    const queries = Array.from({ length: 26 }, () => ({ query: "q" }))
    const full = startDraft(1, dataPanel({ queries }))
    expect(draftReducer(full, { type: "addQuery" })).toBe(full)
  })
})

describe("nextRef", () => {
  it("returns the first letter not used by any default or explicit ref", () => {
    expect(nextRef([{ query: "a" }, { ref: "B", query: "b" }])).toBe("C")
    expect(nextRef([{ ref: "A", query: "a" }, { query: "b" }])).toBe("C")
    expect(nextRef([{ ref: "Z", query: "z" }])).toBe("A")
  })
})

describe("removeQuery", () => {
  it("keeps the refs of later queries explicit", () => {
    const state = apply(
      dataPanel({
        queries: [{ query: "a" }, { query: "b" }, { query: "c" }],
      }),
      { type: "removeQuery", index: 0 }
    )
    expect(queriesOf(state)).toEqual([
      { ref: "B", query: "b" },
      { ref: "C", query: "c" },
    ])
  })

  it("does not touch earlier queries or queries with an explicit ref", () => {
    const state = apply(
      dataPanel({
        queries: [{ query: "a" }, { query: "b" }, { ref: "X", query: "x" }],
      }),
      { type: "removeQuery", index: 1 }
    )
    expect(queriesOf(state)).toEqual([{ query: "a" }, { ref: "X", query: "x" }])
  })

  it("keeps the only query", () => {
    const only = startDraft(1, dataPanel())
    expect(draftReducer(only, { type: "removeQuery", index: 0 })).toBe(only)
  })

  it("ignores an index out of range", () => {
    const two = startDraft(
      1,
      dataPanel({ queries: [{ query: "a" }, { query: "b" }] })
    )
    expect(draftReducer(two, { type: "removeQuery", index: 2 })).toBe(two)
  })
})

describe("setType", () => {
  it("drops display keys the new type does not allow", () => {
    const state = apply(
      dataPanel({ display: { tooltip: "all", legend: "table" } }),
      { type: "setType", panelType: "stat" }
    )
    expect(state.panel.type).toBe("stat")
    expect((state.panel as DataPanel).display).toBeUndefined()
  })

  it("keeps display keys both types allow", () => {
    const state = apply(
      dataPanel({ type: "stat", display: { reduce: "last", graph: true } }),
      { type: "setType", panelType: "gauge" }
    )
    expect((state.panel as DataPanel).display).toEqual({ reduce: "last" })
  })

  it("is ignored on a text panel", () => {
    const text = startDraft(1, textPanel)
    expect(draftReducer(text, { type: "setType", panelType: "stat" })).toBe(
      text
    )
  })
})

describe("setUnit", () => {
  it("sets and removes the unit", () => {
    const set = apply(dataPanel(), { type: "setUnit", unit: "percent" })
    expect((set.panel as DataPanel).unit).toBe("percent")
    const cleared = draftReducer(set, { type: "setUnit", unit: null })
    expect("unit" in cleared.panel).toBe(false)
  })
})

describe("setThresholds", () => {
  const steps = [{ color: "ok" }, { value: 80, color: "warning" }]

  it("sets thresholds", () => {
    const state = apply(dataPanel(), {
      type: "setThresholds",
      thresholds: steps,
    })
    expect((state.panel as DataPanel).thresholds).toEqual(steps)
  })

  it("removes thresholds for null and for an empty list", () => {
    const withSteps = apply(dataPanel({ thresholds: steps }))
    for (const thresholds of [null, []]) {
      const state = draftReducer(withSteps, {
        type: "setThresholds",
        thresholds,
      })
      expect("thresholds" in state.panel).toBe(false)
    }
  })
})

describe("setTitle", () => {
  it("keeps the title as typed", () => {
    const state = apply(dataPanel(), { type: "setTitle", title: "  New " })
    expect(state.panel.title).toBe("  New ")
  })

  it("works on a text panel", () => {
    const state = apply(textPanel, { type: "setTitle", title: "Notes" })
    expect(state.panel.title).toBe("Notes")
  })
})

describe("setDisplay", () => {
  it("sets a key", () => {
    const state = apply(dataPanel(), {
      type: "setDisplay",
      key: "tooltip",
      value: "all",
    })
    expect((state.panel as DataPanel).display).toEqual({ tooltip: "all" })
  })

  it("removes an emptied display object", () => {
    const state = apply(dataPanel({ display: { tooltip: "all" } }), {
      type: "setDisplay",
      key: "tooltip",
      value: undefined,
    })
    expect("display" in state.panel).toBe(false)
  })

  it("keeps the other keys when one is removed", () => {
    const state = apply(
      dataPanel({ display: { tooltip: "all", legend: "list" } }),
      { type: "setDisplay", key: "tooltip", value: undefined }
    )
    expect((state.panel as DataPanel).display).toEqual({ legend: "list" })
  })

  it("is ignored on a text panel", () => {
    const text = startDraft(1, textPanel)
    expect(
      draftReducer(text, { type: "setDisplay", key: "wrap", value: true })
    ).toBe(text)
  })
})

describe("setContent", () => {
  it("changes a text panel", () => {
    const state = apply(textPanel, { type: "setContent", content: "bye" })
    expect((state.panel as TextPanel).content).toBe("bye")
  })

  it("is ignored on a data panel", () => {
    const data = startDraft(1, dataPanel())
    expect(draftReducer(data, { type: "setContent", content: "x" })).toBe(data)
  })
})

describe("replace, reset and rebase", () => {
  it("replaces the draft panel and keeps the base", () => {
    const next = dataPanel({ title: "Other" })
    const state = apply(dataPanel(), { type: "replace", panel: next })
    expect(state.panel).toEqual(next)
    expect(state.panel).not.toBe(next)
    expect(state.base.title).toBe("Requests")
  })

  it("resets the draft to a clone of the base", () => {
    const state = apply(
      dataPanel(),
      { type: "setTitle", title: "Changed" },
      { type: "reset" }
    )
    expect(state.panel).toEqual(state.base)
    expect(state.panel).not.toBe(state.base)
  })

  it("rebases onto a new version", () => {
    const next = dataPanel({ title: "Saved" })
    const state = apply(
      dataPanel(),
      { type: "setTitle", title: "Changed" },
      { type: "rebase", version: 4, panel: next }
    )
    expect(state.baseVersion).toBe(4)
    expect(state.base).toEqual(next)
    expect(state.panel).toEqual(next)
  })
})

describe("immutability", () => {
  const actions: DraftAction[] = [
    { type: "setQuery", index: 0, query: "x" },
    { type: "setLegend", index: 0, legend: "l" },
    { type: "setQueryDatasource", index: 0, uid: "o" },
    { type: "setPanelDatasource", uid: "o" },
    { type: "addQuery" },
    { type: "removeQuery", index: 0 },
    { type: "setType", panelType: "stat" },
    { type: "setUnit", unit: "percent" },
    { type: "setThresholds", thresholds: [{ color: "ok" }] },
    { type: "setTitle", title: "t" },
    { type: "setDisplay", key: "tooltip", value: undefined },
    { type: "setContent", content: "c" },
    { type: "replace", panel: dataPanel() },
    { type: "reset" },
    { type: "rebase", version: 9, panel: dataPanel() },
  ]

  it.each(actions)("$type does not mutate a deeply frozen state", (action) => {
    const state = deepFreeze(
      startDraft(
        1,
        dataPanel({
          queries: [{ query: "a" }, { query: "b" }],
          display: { tooltip: "all" },
          thresholds: [{ color: "ok" }, { value: 1, color: "critical" }],
        })
      )
    )
    const before = stableJson(state)
    expect(() => draftReducer(state, action)).not.toThrow()
    expect(stableJson(state)).toBe(before)
  })
})

describe("displayKeysFor", () => {
  it("lists the display keys of the schema per type", () => {
    expect(displayKeysFor("timeseries")).toEqual([
      "legend",
      "legendValues",
      "tooltip",
    ])
    expect(displayKeysFor("logs")).toEqual(["wrap", "showTime", "order"])
  })
})
