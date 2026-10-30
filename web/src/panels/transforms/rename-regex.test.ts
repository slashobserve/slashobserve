import { describe, expect, it } from "vitest"

import type { PreparedFrame } from "./names"
import { renameByRegex } from "./rename-regex"

const frame: PreparedFrame = {
  ref: "A",
  fields: [
    { name: "Time", type: "time", values: [1] },
    {
      name: "Value",
      type: "number",
      seriesName: "web-1 requests",
      values: [3],
    },
    { name: "Other", type: "number", seriesName: "db-1 errors", values: [4] },
  ],
}

function names(regex: string, renameTo: string): string[] {
  const [out] = renameByRegex([frame], {
    type: "renameByRegex",
    regex,
    renameTo,
  })
  return out.fields.map((f) => f.seriesName ?? f.name)
}

describe("renameByRegex", () => {
  it("renames fields whose name matches, with $1 groups", () => {
    expect(names("(\\w+)-(\\d) requests", "$2:$1")).toEqual([
      "Time",
      "1:web",
      "db-1 errors",
    ])
  })

  it("matches the whole name unless the pattern is written with slashes", () => {
    expect(names("web", "x")).toEqual(["Time", "web-1 requests", "db-1 errors"])
    expect(names("/web/", "x")).toEqual(["Time", "x-1 requests", "db-1 errors"])
  })

  it("matches the field name as well as the series name", () => {
    expect(names("Time", "When")).toEqual([
      "When",
      "web-1 requests",
      "db-1 errors",
    ])
  })

  it("sets the name tables show", () => {
    const [out] = renameByRegex([frame], {
      type: "renameByRegex",
      regex: "Time",
      renameTo: "When",
    })
    expect(out.fields[0].name).toBe("When")
  })

  it("renames to an empty name and ignores an invalid pattern", () => {
    expect(names("Time", "")[0]).toBe("")
    expect(names("(", "x")).toEqual(["Time", "web-1 requests", "db-1 errors"])
    expect(frame.fields[0].name).toBe("Time")
  })
})
