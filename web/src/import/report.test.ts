import { describe, expect, it } from "vitest"

import type { ImportSourceDatasource } from "./errors"
import {
  blockingReason,
  filterCounts,
  filterLines,
  initialFilter,
  statusLabel,
  visibleDatasources,
  type ImportReport,
} from "./report"

function panel(
  id: string,
  status: ImportReport["panels"][number]["status"],
  extra: Partial<ImportReport["panels"][number]> = {}
): ImportReport["panels"][number] {
  return {
    panel_id: id,
    grafana_id: null,
    grafana_path: `panels[${id}]`,
    title: `Panel ${id}`,
    type: "timeseries",
    source_type: "barchart",
    row: "Quick overview",
    status,
    reasons: [],
    paths: [],
    queries: [],
    ...extra,
  }
}

const report: ImportReport = {
  importer_version: 1,
  verified: false,
  counts: { total: 4, imported: 1, partial: 1, placeholder: 1, unverified: 1 },
  panels: [
    panel("a", "imported"),
    panel("b", "partial", {
      reasons: ["Gradient fill is not imported."],
      paths: ["fieldConfig.x", "options.y"],
    }),
    panel("c", "placeholder", { type: "text", row: null }),
    panel("d", "unverified", { reasons: ["The query failed."] }),
  ],
  notes: [
    {
      subject: "Variable env",
      status: "partial",
      reason: "Datasource variables are not imported.",
      paths: [],
    },
  ],
}

const allImported: ImportReport = {
  ...report,
  counts: { total: 1, imported: 1, partial: 0, placeholder: 0, unverified: 0 },
  panels: [panel("a", "imported")],
  notes: [],
}

function datasource(
  extra: Partial<ImportSourceDatasource>
): ImportSourceDatasource {
  return {
    key: "prometheus",
    name: null,
    type: "prometheus",
    kind: "prometheus",
    panels: 3,
    match: "prom",
    matched_by: "alias",
    candidates: ["prom"],
    ...extra,
  }
}

describe("filterCounts", () => {
  it("counts each tab", () => {
    expect(filterCounts(report)).toEqual({
      attention: 3,
      partial: 1,
      placeholder: 1,
      all: 4,
    })
  })
})

describe("initialFilter", () => {
  it("starts on Needs a look when something needs a look", () => {
    expect(initialFilter(report)).toBe("attention")
  })

  it("starts on All panels when every panel imported", () => {
    expect(initialFilter(allImported)).toBe("all")
  })
})

describe("filterLines", () => {
  const keys = (filter: Parameters<typeof filterLines>[1]) =>
    filterLines(report, filter).map((line) => line.key)

  it("attention lists every panel that is not imported, then the notes", () => {
    expect(keys("attention")).toEqual([
      "panel:b",
      "panel:c",
      "panel:d",
      "note:0",
    ])
  })

  it("partial lists partial panels only", () => {
    expect(keys("partial")).toEqual(["panel:b"])
  })

  it("placeholder lists placeholder panels only", () => {
    expect(keys("placeholder")).toEqual(["panel:c"])
  })

  it("all lists every panel in order, then the notes", () => {
    expect(keys("all")).toEqual([
      "panel:a",
      "panel:b",
      "panel:c",
      "panel:d",
      "note:0",
    ])
  })

  it("describes a panel line", () => {
    const [line] = filterLines(report, "partial")
    expect(line).toMatchObject({
      kind: "panel",
      title: "Panel b",
      subtitle: "timeseries · Quick overview",
      reasons: ["Gradient fill is not imported."],
      paths: "fieldConfig.x, options.y",
    })
  })

  it("says imported as is when there are no reasons", () => {
    const line = filterLines(report, "all")[0]
    expect(line.reasons).toEqual(["Imported as is."])
    expect(line.paths).toBe("")
  })

  it("uses the source type for placeholders and drops a missing row", () => {
    const [line] = filterLines(report, "placeholder")
    expect(line.subtitle).toBe("barchart")
  })

  it("describes a note with its subject and no subtitle", () => {
    const note = filterLines(report, "attention").at(-1)
    expect(note).toMatchObject({
      kind: "note",
      title: "Variable env",
      subtitle: null,
      reasons: ["Datasource variables are not imported."],
    })
  })
})

describe("statusLabel", () => {
  it("maps each status to a tone and word", () => {
    expect(statusLabel("imported")).toEqual({ tone: "ok", text: "Imported" })
    expect(statusLabel("partial")).toEqual({ tone: "warning", text: "Partial" })
    expect(statusLabel("placeholder")).toEqual({
      tone: "warning",
      text: "Placeholder",
    })
    expect(statusLabel("unverified")).toEqual({
      tone: "critical",
      text: "Unverified",
    })
  })
})

describe("visibleDatasources", () => {
  it("hides unsupported sources no panel uses", () => {
    const builtin = datasource({ key: "-- Grafana --", kind: null, panels: 0 })
    const unsupported = datasource({ key: "loki", kind: null, panels: 2 })
    const supported = datasource({ panels: 0 })
    expect(visibleDatasources([builtin, unsupported, supported])).toEqual([
      unsupported,
      supported,
    ])
  })
})

describe("blockingReason", () => {
  it("is null when every supported source has a match", () => {
    expect(
      blockingReason([
        datasource({}),
        datasource({ key: "loki", kind: null, match: null }),
      ])
    ).toBeNull()
  })

  it("asks to choose a datasource for the first unmatched source", () => {
    expect(
      blockingReason([
        datasource({ key: "a", match: null, candidates: ["p1", "p2"] }),
        datasource({ key: "b", match: null, candidates: ["p1"] }),
      ])
    ).toBe("Choose a datasource for a.")
  })

  it("says some are not configured when the blocking source has no candidates", () => {
    expect(
      blockingReason([datasource({ key: "a", match: null, candidates: [] })])
    ).toBe("Some datasources are not configured.")
  })
})
