import { describe, expect, it } from "vitest"

import {
  blockHeading,
  blockStatus,
  blockTally,
  implicitText,
  isImplicit,
  omittedText,
  outcomeLabel,
  specText,
  splitOptions,
  tallyText,
  type ImportBlock,
  type ImportOptionLine,
  type Outcome,
} from "./blocks"
import type { ImportReport } from "./report"

function block(
  kind: ImportBlock["kind"],
  extra: Partial<ImportBlock> = {}
): ImportBlock {
  return {
    kind,
    path: "",
    title: "",
    source_type: "",
    panel_id: null,
    outcome: null,
    reason: null,
    options: [],
    implicit: 0,
    omitted: 0,
    blocks: [],
    ...extra,
  }
}

function line(
  outcome: Outcome,
  extra: Partial<ImportOptionLine> = {}
): ImportOptionLine {
  return {
    path: `options.${outcome}`,
    outcome,
    spec: [],
    reason: null,
    ...extra,
  }
}

function report(extra: Partial<ImportReport> = {}): ImportReport {
  return {
    importer_version: 2,
    verified: false,
    counts: {
      total: 0,
      imported: 0,
      partial: 0,
      placeholder: 0,
      unverified: 0,
    },
    panels: [],
    notes: [],
    ...extra,
  }
}

const panelLine: ImportReport["panels"][number] = {
  panel_id: "p1",
  grafana_id: 1,
  grafana_path: "panels[0]",
  title: "Latency",
  type: "timeseries",
  source_type: "graph",
  row: null,
  status: "partial",
  reasons: [],
  paths: [],
  queries: [],
}

describe("outcomeLabel", () => {
  it.each([
    ["mapped", "ok", "Mapped"],
    ["excluded", "none", "Excluded by decision"],
    ["reported", "warning", "Not imported"],
    ["default", "none", "Grafana default"],
    ["no_effect", "none", "No visible effect"],
    ["bookkeeping", "none", "Editor bookkeeping"],
  ] as const)("labels %s", (outcome, tone, text) => {
    expect(outcomeLabel(outcome)).toEqual({ tone, text })
  })
})

describe("isImplicit", () => {
  it.each([
    ["mapped", false],
    ["excluded", false],
    ["reported", false],
    ["default", true],
    ["no_effect", true],
    ["bookkeeping", true],
  ] as const)("%s is implicit: %s", (outcome, expected) => {
    expect(isImplicit(outcome)).toBe(expected)
  })
})

describe("splitOptions", () => {
  it("separates implicit lines and keeps the server's order in each list", () => {
    const options = [
      line("default", { path: "a" }),
      line("mapped", { path: "b" }),
      line("bookkeeping", { path: "c" }),
      line("reported", { path: "d" }),
      line("no_effect", { path: "e" }),
      line("excluded", { path: "f" }),
    ]
    const { main, implicit } = splitOptions(block("panel", { options }))
    expect(main.map((item) => item.path)).toEqual(["b", "d", "f"])
    expect(implicit.map((item) => item.path)).toEqual(["a", "c", "e"])
  })
})

describe("blockHeading", () => {
  it.each([
    [block("dashboard"), "Dashboard settings", null],
    [
      block("variable", { title: "env", source_type: "query" }),
      "Variable env",
      "query",
    ],
    [block("row", { title: "Overview" }), "Row Overview", null],
    [
      block("panel", { title: "Latency", source_type: "graph" }),
      "Latency",
      "graph",
    ],
    [block("panel"), "Untitled panel", null],
    [
      block("query", { title: "A", source_type: "prometheus" }),
      "Query A",
      "prometheus",
    ],
    [
      block("transformation", { title: "organize" }),
      "Transformation organize",
      null,
    ],
    [block("field_config"), "Field config", null],
    [block("override", { title: "byName" }), "Override byName", null],
  ])("titles %#", (input, title, subtitle) => {
    expect(blockHeading(input)).toEqual({ title, subtitle })
  })
})

describe("blockTally", () => {
  it("sums the block and its descendants", () => {
    const child = block("query", {
      options: [line("mapped"), line("reported")],
      implicit: 2,
      omitted: 1,
    })
    const nested = block("field_config", {
      options: [line("excluded"), line("mapped")],
      implicit: 1,
      blocks: [child],
    })
    const root = block("panel", {
      options: [line("mapped"), line("default")],
      implicit: 1,
      blocks: [nested],
    })
    expect(blockTally(root)).toEqual({
      mapped: 3,
      excluded: 1,
      reported: 1,
      implicit: 4,
      omitted: 1,
    })
  })

  it("counts a child block with its own outcome once", () => {
    const root = block("panel", {
      blocks: [
        block("transformation", { outcome: "excluded", reason: "Left out." }),
        block("transformation", { outcome: "reported", reason: "Not yet." }),
      ],
    })
    expect(blockTally(root)).toEqual({
      mapped: 0,
      excluded: 1,
      reported: 1,
      implicit: 0,
      omitted: 0,
    })
  })
})

describe("tallyText", () => {
  it("drops zero parts", () => {
    expect(
      tallyText({
        mapped: 3,
        excluded: 0,
        reported: 2,
        implicit: 0,
        omitted: 0,
      })
    ).toBe("3 mapped · 2 not imported")
  })

  it("lists every part in order", () => {
    expect(
      tallyText({
        mapped: 1,
        excluded: 2,
        reported: 3,
        implicit: 4,
        omitted: 0,
      })
    ).toBe("1 mapped · 2 excluded · 3 not imported · 4 need no setting")
  })

  it("says there are no options when all parts are zero", () => {
    expect(
      tallyText({
        mapped: 0,
        excluded: 0,
        reported: 0,
        implicit: 0,
        omitted: 5,
      })
    ).toBe("No options")
  })
})

describe("blockStatus", () => {
  it("uses the panel line with the same panel id", () => {
    const status = blockStatus(
      block("panel", { panel_id: "p1" }),
      report({ panels: [panelLine] })
    )
    expect(status).toEqual({ tone: "warning", text: "Partial" })
  })

  it("uses the panel line for a row block too", () => {
    const status = blockStatus(
      block("row", { panel_id: "p1" }),
      report({ panels: [{ ...panelLine, status: "unverified" }] })
    )
    expect(status).toEqual({ tone: "critical", text: "Unverified" })
  })

  it("is null when the panel line is missing", () => {
    expect(
      blockStatus(
        block("panel", { panel_id: "zz" }),
        report({ panels: [panelLine] })
      )
    ).toBeNull()
  })

  it("marks a variable unverified by the note that lists its path", () => {
    const note = {
      subject: "Variable env",
      status: "unverified" as const,
      reason: "The query failed.",
      paths: ["templating.list[1]"],
    }
    const variable = block("variable", { path: "templating.list[1]" })
    expect(blockStatus(variable, report({ notes: [note] }))).toEqual({
      tone: "critical",
      text: "Unverified",
    })
    expect(
      blockStatus(
        block("variable", { path: "templating.list[0]" }),
        report({ notes: [note] })
      )
    ).toBeNull()
  })

  it("ignores a note on the variable's path that is not unverified", () => {
    const note = {
      subject: "Variable env",
      status: "partial" as const,
      reason: "Skipped.",
      paths: ["templating.list[1]"],
    }
    expect(
      blockStatus(
        block("variable", { path: "templating.list[1]" }),
        report({ notes: [note] })
      )
    ).toBeNull()
  })

  it("marks the dashboard unverified by a dashboard note", () => {
    const note = {
      subject: "dashboard",
      status: "unverified" as const,
      reason: "A query failed.",
      paths: [],
    }
    expect(blockStatus(block("dashboard"), report({ notes: [note] }))).toEqual({
      tone: "critical",
      text: "Unverified",
    })
    expect(blockStatus(block("dashboard"), report())).toBeNull()
  })

  it("is null for a variable with no note", () => {
    expect(
      blockStatus(block("variable", { path: "templating.list[0]" }), report())
    ).toBeNull()
  })

  it("is null for kinds without a status", () => {
    expect(
      blockStatus(block("query"), report({ panels: [panelLine] }))
    ).toBeNull()
  })
})

describe("specText", () => {
  it("lists the Spec fields of a mapped option", () => {
    expect(
      specText(line("mapped", { spec: ["fieldConfig.unit", "display.legend"] }))
    ).toBe("→ fieldConfig.unit, display.legend")
  })

  it("says when no Spec field is listed", () => {
    expect(specText(line("mapped"))).toBe("Spec field not listed")
  })

  it("gives the reason of excluded and reported options", () => {
    expect(
      specText(line("excluded", { reason: "Left out by decision." }))
    ).toBe("Left out by decision.")
    expect(specText(line("reported", { reason: null }))).toBe("")
  })

  it.each(["default", "no_effect", "bookkeeping"] as const)(
    "is empty for %s",
    (outcome) => {
      expect(specText(line(outcome, { reason: "ignored" }))).toBe("")
    }
  )
})

describe("implicitText", () => {
  it.each([
    [0, null],
    [1, "1 option needs no setting"],
    [7, "7 options need no setting"],
  ])("for %i", (implicit, expected) => {
    expect(implicitText(block("panel", { implicit }))).toBe(expected)
  })
})

describe("omittedText", () => {
  it.each([
    [0, null],
    [1, "1 more option is not listed"],
    [12, "12 more options are not listed"],
  ])("for %i", (omitted, expected) => {
    expect(omittedText(block("panel", { omitted }))).toBe(expected)
  })
})
