import { describe, expect, it } from "vitest"

import { sameJson, stableJson } from "@/lib/json"
import {
  dataPanelTypes,
  type DashlistPanel,
  type DataPanel,
  type NewsPanel,
  type PanelQuery,
  type TextPanel,
} from "@/spec"

import schemaText from "../../../spec/schema/observespec.v0.json?raw"

import {
  displayKeysFor,
  draftReducer,
  isEditablePanel,
  nextRef,
  startDraft,
  type DraftAction,
  type EditablePanel,
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

function apply(panel: EditablePanel, ...actions: DraftAction[]) {
  return actions.reduce(
    (state, action) => draftReducer(state, action),
    startDraft(3, panel)
  )
}

function queriesOf(state: EditorDraft): PanelQuery[] {
  return (state.panel as DataPanel).queries
}

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
  it("accepts data, text and dashboard list panels only", () => {
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

  it("stores a copy, so editing the caller's steps leaves the draft alone", () => {
    const given = [{ color: "ok" }, { value: 80, color: "warning" }]
    const state = apply(dataPanel(), {
      type: "setThresholds",
      thresholds: given,
    })
    given[1].value = 99
    given.push({ value: 100, color: "critical" })
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

  it("removes the title again when it is cleared on a panel saved without one", () => {
    const untitled: Partial<DataPanel> = dataPanel()
    delete untitled.title
    const state = apply(
      untitled as DataPanel,
      { type: "setTitle", title: "x" },
      { type: "setTitle", title: "" }
    )
    expect("title" in state.panel).toBe(false)
    expect(sameJson(state.panel, state.base)).toBe(true)
  })

  it("leaves a panel saved without a title unchanged when only whitespace is typed", () => {
    const untitled: Partial<DataPanel> = dataPanel()
    delete untitled.title
    const state = apply(untitled as DataPanel, {
      type: "setTitle",
      title: "   ",
    })
    expect("title" in state.panel).toBe(false)
    expect(sameJson(state.panel, state.base)).toBe(true)
  })

  it("keeps an empty title on a panel saved with one", () => {
    const state = apply(dataPanel(), { type: "setTitle", title: "" })
    expect(state.panel.title).toBe("")
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

  it("stores a copy, so editing the caller's value leaves the draft alone", () => {
    const given = ["mean", "max"]
    const state = apply(dataPanel(), {
      type: "setDisplay",
      key: "legendValues",
      value: given,
    })
    given.push("min")
    expect((state.panel as DataPanel).display).toEqual({
      legendValues: ["mean", "max"],
    })
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

describe("dashlist drafts", () => {
  const dashlist: DashlistPanel = { id: "d1", type: "dashlist", gridPos }

  it("is editable", () => {
    expect(isEditablePanel(dashlist)).toBe(true)
  })

  it("takes setDisplay and removes the key again with undefined", () => {
    const set = apply(dashlist, {
      type: "setDisplay",
      key: "tags",
      value: ["prod"],
    })
    expect((set.panel as DashlistPanel).display).toEqual({ tags: ["prod"] })
    const cleared = draftReducer(set, {
      type: "setDisplay",
      key: "tags",
      value: undefined,
    })
    expect("display" in cleared.panel).toBe(false)
  })

  it("ignores setDisplay on a text panel", () => {
    const state = apply(textPanel, {
      type: "setDisplay",
      key: "tags",
      value: ["prod"],
    })
    expect(state.panel).toEqual(textPanel)
  })
})

describe("news drafts", () => {
  const news: NewsPanel = { id: "n1", type: "news", gridPos }

  it("is editable", () => {
    expect(isEditablePanel(news)).toBe(true)
  })

  it("takes setDisplay for the feed URL and removes it again with undefined", () => {
    const set = apply(news, {
      type: "setDisplay",
      key: "feedUrl",
      value: "https://example.com/feed.xml",
    })
    expect((set.panel as NewsPanel).display).toEqual({
      feedUrl: "https://example.com/feed.xml",
    })
    const cleared = draftReducer(set, {
      type: "setDisplay",
      key: "feedUrl",
      value: undefined,
    })
    expect("display" in cleared.panel).toBe(false)
  })

  it("takes setTitle", () => {
    const state = apply(news, { type: "setTitle", title: "Blog" })
    expect(state.panel.title).toBe("Blog")
  })
})

describe("setType display keys", () => {
  it("drops canvas elements and connections when the type changes to stat", () => {
    const state = apply(
      dataPanel({
        type: "canvas",
        display: {
          reduce: "max",
          elements: [{ id: "e", type: "text", x: 0, y: 0, w: 1, h: 1 }],
          connections: [],
        },
      } as Partial<DataPanel>),
      { type: "setType", panelType: "stat" }
    )
    const panel = state.panel as DataPanel
    expect(panel.type).toBe("stat")
    expect(panel.display).toEqual({ reduce: "max" })
  })
})

describe("displayKeysFor", () => {
  type Schema = {
    $defs: {
      dataPanel: {
        allOf: {
          if?: { properties: { type: { const: string } } }
          then?: { properties: { display: { properties: object } } }
        }[]
      }
    }
  }
  const schema = JSON.parse(schemaText) as Schema
  const schemaKeys = new Map(
    schema.$defs.dataPanel.allOf.flatMap((rule) =>
      rule.if && rule.then
        ? [
            [
              rule.if.properties.type.const,
              Object.keys(rule.then.properties.display.properties).sort(),
            ] as const,
          ]
        : []
    )
  )

  it("lists the display keys of the schema for every data panel type", () => {
    expect(dataPanelTypes).toHaveLength(21)
    expect([...schemaKeys.keys()].sort()).toEqual([...dataPanelTypes].sort())
    for (const type of dataPanelTypes) {
      expect([...displayKeysFor(type)].sort(), type).toEqual(
        schemaKeys.get(type)
      )
    }
  })
})
