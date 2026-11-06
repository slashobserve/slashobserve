import { describe, expect, it } from "vitest"

import { createFieldResolver } from "@/panels/field-config"
import type { PreparedFrame } from "@/panels/transforms/names"
import type { DataPanel } from "@/spec"

import { formattedCell, framesToCsv, framesToTable, panelFileName } from "./csv"

const t1 = Date.parse("2026-11-06T10:00:00Z")
const t2 = Date.parse("2026-11-06T10:01:00Z")
const t3 = Date.parse("2026-11-06T10:02:00Z")

function series(
  ref: string,
  name: string,
  times: number[],
  values: (number | null)[]
): PreparedFrame {
  return {
    ref,
    fields: [
      { name: "Time", type: "time", values: times },
      { name: "Value", type: "number", values, seriesName: name },
    ],
  }
}

function rows(csv: string): string[] {
  return csv.split("\r\n")
}

describe("framesToCsv", () => {
  it("writes nothing for no frames", () => {
    expect(framesToCsv([])).toBe("")
  })

  it("writes only the header for a frame without rows", () => {
    expect(framesToCsv([series("A", "cpu", [], [])])).toBe("Time,cpu\r\n")
  })

  it("writes one row per point with times in RFC 3339 UTC", () => {
    expect(
      rows(framesToCsv([series("A", "cpu", [t1, t2], [0.5, null])]))
    ).toEqual([
      "Time,cpu",
      "2026-11-06T10:00:00.000Z,0.5",
      "2026-11-06T10:01:00.000Z,",
      "",
    ])
  })

  it("quotes commas, quotes and line breaks as RFC 4180 says", () => {
    const frame: PreparedFrame = {
      ref: "A",
      fields: [
        { name: "pod, zone", type: "string", values: ['say "hi"', "a\nb"] },
        { name: "ok", type: "bool", values: [true, false] },
        { name: "raw", type: "other", values: [{ a: 1 }, null] },
      ],
    }
    expect(framesToCsv([frame])).toBe(
      '"pod, zone",ok,raw\r\n' +
        '"say ""hi""",true,"{""a"":1}"\r\n' +
        '"a\nb",false,\r\n'
    )
  })

  it("joins frames on time when every frame has unique times", () => {
    const csv = framesToCsv([
      series("A", 'up{pod="a"}', [t1, t2], [1, 2]),
      series("B", 'up{pod="b"}', [t2, t3], [3, 4]),
    ])
    expect(rows(csv)).toEqual([
      'Time,"up{pod=""a""}","up{pod=""b""}"',
      "2026-11-06T10:00:00.000Z,1,",
      "2026-11-06T10:01:00.000Z,2,3",
      "2026-11-06T10:02:00.000Z,,4",
      "",
    ])
  })

  it("concatenates frames with a series column when they share no time", () => {
    const table = (
      ref: string,
      name: string,
      pods: string[]
    ): PreparedFrame => ({
      ref,
      name,
      fields: [{ name: "pod", type: "string", values: pods }],
    })
    expect(
      rows(
        framesToCsv([
          table("A", "first", ["a"]),
          table("B", "second", ["b", "c"]),
        ])
      )
    ).toEqual(["series,pod", "first,a", "second,b", "second,c", ""])
  })

  it("concatenates frames whose times repeat, so no row is lost", () => {
    const logs = (ref: string, lines: string[]): PreparedFrame => ({
      ref,
      fields: [
        { name: "Time", type: "time", values: lines.map(() => t1) },
        { name: "Line", type: "string", values: lines },
      ],
    })
    expect(
      rows(framesToCsv([logs("A", ["x", "y"]), logs("B", ["z"])]))
    ).toEqual([
      "series,Time,Line",
      "A,2026-11-06T10:00:00.000Z,x",
      "A,2026-11-06T10:00:00.000Z,y",
      "B,2026-11-06T10:00:00.000Z,z",
      "",
    ])
  })

  it("names a concatenated series by its drawn name and leaves missing fields empty", () => {
    const withValue = series("A", "cpu", [t1], [1])
    const tableOnly: PreparedFrame = {
      ref: "B",
      fields: [{ name: "note", type: "string", values: ["n"] }],
    }
    expect(rows(framesToCsv([withValue, tableOnly]))).toEqual([
      "series,Time,Value,note",
      "cpu,2026-11-06T10:00:00.000Z,1,",
      "B,,,n",
      "",
    ])
  })
})

describe("framesToTable", () => {
  const frames = [
    series("A", "cpu", [t1, t2], [0.30000000000000004, 1]),
    series("B", "mem", [t1, t2], [2, 3]),
  ]

  it("keeps raw values without a format", () => {
    expect(
      framesToTable([series("A", "cpu", [t1], [0.30000000000000004])])
    ).toEqual({
      columns: ["Time", "cpu"],
      rows: [["2026-11-06T10:00:00.000Z", "0.30000000000000004"]],
    })
  })

  it("passes every cell through the format and keeps the column names", () => {
    const table = framesToTable(frames, () => "x")
    expect(table.columns).toEqual(["Time", "cpu", "mem"])
    expect(table.rows).toEqual([
      ["x", "x", "x"],
      ["x", "x", "x"],
    ])
  })

  it("hands the format the frame and field each cell came from", () => {
    const table = framesToTable(
      frames,
      (frame, field, value) => `${frame.ref}.${field.name}=${String(value)}`
    )
    expect(table.rows[1]).toEqual([`A.Time=${t2}`, "A.Value=1", "B.Value=3"])
  })
})

describe("formattedCell", () => {
  const panel = {
    id: "p",
    title: "p",
    type: "timeseries",
    gridPos: { x: 0, y: 0, w: 6, h: 4 },
    datasource: "prom",
    queries: [{ query: "a" }],
    unit: "percent",
    decimals: 1,
    mappings: [{ value: "100", text: "Full" }],
  } as DataPanel
  const format = formattedCell(createFieldResolver(panel), "Asia/Kolkata")

  it("shows times in the dashboard zone and values in the panel's unit", () => {
    const table = framesToTable(
      [series("A", "cpu", [t1, t2], [0.30000000000000004, 100])],
      format
    )
    expect(table.rows).toEqual([
      ["2026-11-06 15:30:00", "0.3%"],
      ["2026-11-06 15:31:00", "Full"],
    ])
  })
})

describe("panelFileName", () => {
  it.each([
    ["Requests per second", "csv", "requests-per-second.csv"],
    ["///", "png", "panel.png"],
  ])("names %s.%s as %s", (title, extension, expected) => {
    expect(panelFileName(title, extension)).toBe(expected)
  })
})
