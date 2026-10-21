import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import type { WireFrame } from "@/dashboards/queries"
import { createFieldResolver } from "@/panels/field-config"
import type { PreparedFrame } from "@/panels/transforms"
import type { DataPanel } from "@/spec"

import {
  cellLinks,
  cellOf,
  tableModel,
  type TableColumn,
  type TableModel,
} from "./table-model"

beforeEach(() => {
  vi.stubGlobal("location", { origin: "https://observe.example.com" })
})

afterEach(() => {
  vi.unstubAllGlobals()
})

function panel(extra: Partial<DataPanel> = {}): DataPanel {
  return {
    id: "t1",
    title: "Table",
    gridPos: { x: 0, y: 0, w: 12, h: 8 },
    type: "table",
    datasource: "prom",
    queries: [{ query: "up" }],
    ...extra,
  } as DataPanel
}

function frame(
  fields: WireFrame["fields"],
  extra: Partial<Omit<WireFrame, "fields">> = {}
): PreparedFrame {
  return { ref: "A", fields, ...extra }
}

const time: WireFrame["fields"][number] = {
  name: "Time",
  type: "time",
  values: [1, 2],
}

const number = (values: (number | null)[], name = "Value") => ({
  name,
  type: "number" as const,
  values,
})

const text = (values: (string | null)[], name: string) => ({
  name,
  type: "string" as const,
  values,
})

function build(
  frames: PreparedFrame[],
  extra: Partial<DataPanel> = {},
  display: Record<string, unknown> = {}
) {
  const fields = createFieldResolver(panel(extra))
  const model = tableModel(frames, fields, display)
  const column = (label: string): TableColumn => {
    const found = model.columns.find((candidate) => candidate.label === label)
    if (!found) throw new Error(`no column "${label}"`)
    return found
  }
  const cell = (label: string, row: number) =>
    cellOf(fields, column(label), model.rows[row])
  const links = (label: string, row: number) =>
    cellLinks(fields, column(label), model.rows[row], cell(label, row), {
      vars: {},
      fields,
    })
  return { fields, model, column, cell, links }
}

const labels = (model: TableModel) => model.columns.map((c) => c.label)

const podFrames = (ref: string, values: number[]) =>
  values.map((value, index) =>
    frame([time, number([value])], {
      ref,
      labels: { pod: `${ref.toLowerCase()}${index}` },
    })
  )

describe("table columns", () => {
  it("drops hidden columns", () => {
    const { model } = build(
      [frame([text(["x"], "route"), number([1], "n"), number([2], "m")])],
      { overrides: [{ match: { field: "m" }, hidden: true }] }
    )
    expect(labels(model)).toEqual(["route", "n"])
  })

  it("heads an unlabeled column with its display name when the config sets one", () => {
    const { model } = build([frame([text(["x"], "route"), number([1], "n")])], {
      overrides: [{ match: { field: "n" }, displayName: "Requests" }],
    })
    expect(labels(model)).toEqual(["route", "Requests"])
  })

  it("heads label columns with the label key even when a display name is set", () => {
    const { model } = build(podFrames("A", [1, 2]), {
      displayName: "Requests",
    })
    expect(labels(model)).toEqual(["pod", "Requests"])
  })

  it("falls back to the field name when frames resolve different display names", () => {
    const { model } = build(podFrames("A", [1, 2]), {
      displayName: "${__field.labels.pod}",
    })
    expect(labels(model)).toEqual(["pod", "Value"])
  })

  it("keeps the display name when only some frames carry the column", () => {
    const first = frame([time, number([1])], { labels: { pod: "a" } })
    const second = frame([time, number([2], "Other")], { labels: { pod: "b" } })
    const { model } = build([first, second], {
      overrides: [{ match: { field: "Value" }, displayName: "Requests" }],
    })
    expect(labels(model)).toEqual(["pod", "Requests", "Other"])
  })

  it("adds a Query column when several queries answer", () => {
    const { model } = build([
      frame([number([1], "n")], { ref: "A" }),
      frame([number([2], "n")], { ref: "B" }),
    ])
    expect(labels(model)).toEqual(["Query", "n"])
    expect(model.rows.map((row) => row.values["query:ref"])).toEqual(["A", "B"])
  })

  it("keeps duplicate field names as separate columns", () => {
    const { model } = build([frame([number([1, 2], "x"), number([3, 4], "x")])])
    expect(labels(model)).toEqual(["x", "x"])
    expect(model.rows.map((row) => Object.values(row.values))).toEqual([
      [1, 3],
      [2, 4],
    ])
  })

  it("gives every row a distinct key", () => {
    const unlabeled = build([
      frame([number([1, 2], "n")]),
      frame([number([3], "n")]),
    ])
    expect(new Set(unlabeled.model.rows.map((row) => row.key)).size).toBe(3)
    const labeled = build(podFrames("A", [1, 2, 3]))
    expect(new Set(labeled.model.rows.map((row) => row.key)).size).toBe(3)
  })
})

describe("table footer", () => {
  it("sums and averages a column with decimals shared across rows", () => {
    const { model } = build(
      [
        frame([number([10.5])], { labels: { pod: "a" } }),
        frame([number([20])], { labels: { pod: "b" } }),
        frame([number([3.25])], { labels: { pod: "c" } }),
      ],
      { unit: "percent" },
      { footer: ["sum", "mean"] }
    )
    expect(model.footer).toEqual([
      { label: "Total", cells: [null, "33.75%"] },
      { label: "Mean", cells: [null, "11.25%"] },
    ])
  })

  it("counts rows without the column unit", () => {
    const { model } = build(
      [
        frame([number([1])], { labels: { pod: "a" } }),
        frame([number([2])], { labels: { pod: "b" } }),
        frame([number([3])], { labels: { pod: "c" } }),
      ],
      { unit: "bytes" },
      { footer: ["count"] }
    )
    expect(model.footer).toEqual([{ label: "Count", cells: [null, "3"] }])
  })
})

describe("table cells", () => {
  const thresholds = [
    { color: "ok" as const },
    { value: 10, color: "warning" as const },
  ]
  const rows = frame([number([5, 50, null], "n")])

  it("shows every row of a column with the decimals the column shares", () => {
    const { cell } = build(
      [
        frame([number([20])], { labels: { pod: "a" } }),
        frame([number([3.25])], { labels: { pod: "b" } }),
      ],
      { unit: "percent" }
    )
    expect(cell("Value", 0).text).toBe("20.00%")
    expect(cell("Value", 1).text).toBe("3.25%")
  })

  it("colors a cell by its threshold and names the level", () => {
    const { cell } = build([rows], { thresholds })
    expect(cell("n", 0)).toMatchObject({ level: "ok", word: "OK" })
    expect(cell("n", 1)).toMatchObject({
      text: "50",
      level: "warning",
      word: "Warning",
      color: "var(--threshold-warning)",
    })
  })

  it("colors a cell by a mapping color alone", () => {
    const { cell } = build([rows], {
      mappings: [{ range: [0, 10], text: "low", color: "info" }],
    })
    expect(cell("n", 0)).toEqual({
      text: "low",
      color: "var(--threshold-info)",
      level: "base",
      word: "",
    })
    expect(cell("n", 1)).toEqual({
      text: "50",
      color: null,
      level: null,
      word: "",
    })
  })

  it("shows the no-value text without a color for a null", () => {
    const { cell } = build([rows], { thresholds, noValue: "n/a" })
    expect(cell("n", 2)).toEqual({
      text: "n/a",
      color: null,
      level: null,
      word: "",
    })
  })

  it("leaves a column without thresholds or colored mappings uncolored", () => {
    const { cell } = build([rows], {
      mappings: [{ range: [0, 10], text: "low" }],
    })
    expect(cell("n", 0)).toEqual({
      text: "low",
      color: null,
      level: null,
      word: "",
    })
    expect(cell("n", 1).color).toBeNull()
  })

  it("takes the config of a label column from a fieldType string override", () => {
    const { cell } = build(podFrames("A", [1, 2]), {
      overrides: [
        {
          match: { fieldType: "string" },
          mappings: [{ value: "a0", text: "Alpha" }],
        },
      ],
    })
    expect(cell("pod", 0).text).toBe("Alpha")
    expect(cell("pod", 1).text).toBe("a1")
  })
})

describe("table links", () => {
  const links = [
    {
      title: "Open ${__data.fields.route}",
      url: "/routes?name=${__data.fields.route}",
    },
  ]

  it("resolves row fields in an unlabeled table", () => {
    const { links: cellLinksOf } = build(
      [frame([text(["/cart", "/pay"], "route"), number([1, 2], "n")])],
      { links }
    )
    expect(cellLinksOf("n", 1)).toEqual([
      {
        title: "Open /pay",
        href: "/routes?name=%2Fpay",
        newTab: false,
      },
    ])
  })

  it("resolves the frame's last values in a labeled table", () => {
    const withRoute = (route: string, value: number) =>
      frame([time, text([route], "route"), number([value], "n")], {
        labels: { pod: route.slice(1) },
      })
    const { links: cellLinksOf } = build(
      [withRoute("/cart", 1), withRoute("/pay", 2)],
      { links }
    )
    expect(cellLinksOf("n", 1)[0].href).toBe("/routes?name=%2Fpay")
    expect(cellLinksOf("pod", 0)[0].href).toBe("/routes?name=%2Fcart")
  })
})

describe("table rows from several queries", () => {
  const frames = [...podFrames("A", [90, 120]), ...podFrames("B", [2048, 4096])]

  it("formats each row with the unit of its own query", () => {
    const { cell } = build(frames, {
      unit: "s",
      overrides: [{ match: { query: "B" }, unit: "bytes" }],
    })
    expect(cell("Value", 0).text).toBe("1.5 min")
    expect(cell("Value", 2).text).toBe("2 KiB")
    expect(cell("Value", 3).text).toBe("4 KiB")
  })

  it("formats each unlabeled row with the unit of its own query", () => {
    const { cell } = build(
      [
        frame([number([90], "n")], { ref: "A" }),
        frame([number([2048], "n")], { ref: "B" }),
      ],
      {
        unit: "s",
        overrides: [{ match: { query: "B" }, unit: "bytes" }],
      }
    )
    expect(cell("n", 0).text).toBe("1.5 min")
    expect(cell("n", 1).text).toBe("2 KiB")
  })

  it("colors and links only the rows whose query configures them", () => {
    const { cell, links } = build(frames, {
      overrides: [
        {
          match: { query: "B" },
          thresholds: [{ color: "ok" }, { value: 3000, color: "critical" }],
          links: [{ title: "Pod ${__data.fields.Value}", url: "/pods/b" }],
        },
      ],
    })
    expect(cell("Value", 0).level).toBeNull()
    expect(cell("Value", 2).level).toBe("ok")
    expect(cell("Value", 3).level).toBe("critical")
    expect(links("Value", 0)).toEqual([])
    expect(links("Value", 3)).toHaveLength(1)
  })

  it("matches a series override to its own row only", () => {
    const { cell } = build(frames, {
      overrides: [
        {
          match: { series: '{pod="a1"}' },
          mappings: [{ range: [0, 1000], text: "slow" }],
        },
      ],
    })
    expect(cell("Value", 0).text).toBe("90")
    expect(cell("Value", 1).text).toBe("slow")
  })
})
