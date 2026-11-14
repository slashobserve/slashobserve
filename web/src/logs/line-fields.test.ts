import { describe, expect, it } from "vitest"

import type { LogEntry } from "@/panels/logs/logs-model"

import {
  columnList,
  columnsText,
  defaultColumns,
  fieldNames,
  lineId,
  linePermalink,
  moveColumn,
} from "./line-fields"
import { validateLogsSearch } from "./search"

function entry(names: string[], extra: Partial<LogEntry> = {}): LogEntry {
  return {
    key: "A|1762438796641|acquirer-b returned 503",
    time: 1762438796641,
    level: null,
    message: "acquirer-b returned 503",
    labels: {},
    repeats: 1,
    details: names.map((name) => ({ name, text: "x" })),
    ...extra,
  }
}

describe("fieldNames", () => {
  it("lists the most frequent field first and breaks ties alphabetically", () => {
    const entries = [
      entry(["pod", "service", "region"]),
      entry(["service", "region"]),
      entry(["service", "zone"]),
    ]
    expect(fieldNames(entries)).toEqual(["service", "region", "pod", "zone"])
  })

  it("reads at most max entries", () => {
    const entries = [entry(["a"]), entry(["b"]), entry(["c"])]
    expect(fieldNames(entries, 2)).toEqual(["a", "b"])
  })

  it.each(["Time", "TimeNs", "Body", "message", "Line", "level"])(
    "leaves out %s, which the line already shows",
    (name) => {
      expect(fieldNames([entry([name, "pod"])])).toEqual(["pod"])
    }
  )

  it("leaves out a name with a comma, which the address cannot carry", () => {
    expect(fieldNames([entry(["a,b", "pod"])])).toEqual(["pod"])
  })

  it("returns nothing for no entries", () => {
    expect(fieldNames([])).toEqual([])
  })
})

describe("columns in the address", () => {
  it("reads an absent value as no columns", () => {
    expect(columnList(undefined)).toEqual([])
  })

  it("splits and joins on commas", () => {
    expect(columnList("service,pod")).toEqual(["service", "pod"])
    expect(columnsText(["service", "pod"])).toBe("service,pod")
  })

  it("writes no columns as an absent value", () => {
    expect(columnsText([])).toBeUndefined()
  })

  it("writes a value the page's own validation keeps", () => {
    const cols = columnsText(["service", "Fields.http.status"])
    expect(validateLogsSearch({ cols }).cols).toBe(cols)
  })
})

describe("defaultColumns", () => {
  it.each([
    ["elasticsearch", ["service", "pod"], ["service"]],
    ["opensearch", ["service"], ["service"]],
    ["elasticsearch", ["pod"], []],
    ["loki", ["service", "pod"], []],
    ["clickhouse", ["service"], []],
  ])("for %s with fields %j gives %j", (type, names, expected) => {
    expect(defaultColumns(type, names)).toEqual(expected)
  })
})

describe("moveColumn", () => {
  it.each([
    ["service", 1, ["pod", "service", "zone"]],
    ["zone", -1, ["service", "zone", "pod"]],
    ["service", -1, ["service", "pod", "zone"]],
    ["zone", 1, ["service", "pod", "zone"]],
    ["missing", 1, ["service", "pod", "zone"]],
  ] as const)("moving %s by %i gives %j", (name, step, expected) => {
    expect(moveColumn(["service", "pod", "zone"], name, step)).toEqual(expected)
  })

  it("does not change its input", () => {
    const names = ["a", "b"]
    moveColumn(names, "a", 1)
    expect(names).toEqual(["a", "b"])
  })
})

describe("lineId", () => {
  it("is the time and a hash, in the shape the address accepts", () => {
    const id = lineId(entry([]))
    expect(id).toMatch(/^1762438796641:[a-z0-9]+$/)
    expect(validateLogsSearch({ line: id }).line).toBe(id)
  })

  it("is the same for the same line and differs for another", () => {
    const first = lineId(entry([]))
    expect(lineId(entry([]))).toBe(first)
    expect(lineId(entry([], { key: "A|1762438796641|other" }))).not.toBe(first)
  })

  it("fits the address for a key of any length or characters", () => {
    const id = lineId(entry([], { key: `A|1|${'é\n"{}'.repeat(500)}` }))
    expect(validateLogsSearch({ line: id }).line).toBe(id)
  })

  it("is null for a line without a time", () => {
    expect(lineId(entry([], { time: null }))).toBeNull()
  })
})

describe("linePermalink", () => {
  it("adds the line to the address and keeps the rest", () => {
    const id = lineId(entry([]))!
    const link = new URL(
      linePermalink("http://localhost/logs/loki?q=%7Ba%7D", entry([]))
    )
    expect(link.searchParams.get("line")).toBe(id)
    expect(link.pathname).toBe("/logs/loki")
    expect(link.searchParams.get("q")).toBe("{a}")
  })

  it("replaces a line already in the address", () => {
    const link = new URL(
      linePermalink("http://localhost/logs/loki?line=1%3Aold", entry([]))
    )
    expect(link.searchParams.getAll("line")).toEqual([lineId(entry([]))])
  })

  it("keeps the address as it is for a line without a time", () => {
    const href = "http://localhost/logs/loki"
    expect(linePermalink(href, entry([], { time: null }))).toBe(href)
  })
})
