import { describe, expect, it } from "vitest"

import { convertFieldType } from "./convert-type"
import type { PreparedField, PreparedFrame } from "./names"

function frameOf(...fields: PreparedField[]): PreparedFrame {
  return { ref: "A", fields }
}

function one(
  values: unknown[],
  conversion: Parameters<typeof convertFieldType>[1]["conversions"][number],
  type: PreparedField["type"] = "string"
): PreparedField {
  const [out] = convertFieldType([frameOf({ name: "x", type, values })], {
    type: "convertFieldType",
    conversions: [conversion],
  })
  return out.fields[0]
}

describe("convertFieldType", () => {
  it("turns text into numbers and unparseable values into null", () => {
    expect(
      one(["1", "2.5", "", "abc", null, "Infinity"], {
        field: "x",
        to: "number",
      })
    ).toEqual({
      name: "x",
      type: "number",
      values: [1, 2.5, null, null, null, null],
    })
  })

  it("turns values into text and joins lists with joinWith", () => {
    expect(
      one(
        [1, true, ["a", "b"], null],
        { field: "x", to: "string", joinWith: " | " },
        "other"
      ).values
    ).toEqual(["1", "true", "a | b", null])
    expect(
      one([["a", "b"]], { field: "x", to: "string" }, "other").values
    ).toEqual(["a,b"])
  })

  it("reads time from milliseconds, ISO text and a date format", () => {
    expect(
      one([1700000000000, "2026-10-07T15:04:09Z", "junk", {}], {
        field: "x",
        to: "time",
      })
    ).toEqual({
      name: "x",
      type: "time",
      values: [1700000000000, Date.UTC(2026, 9, 7, 15, 4, 9), null, null],
    })
    const local = new Date(2026, 9, 7, 15, 4).getTime()
    expect(
      one(["07/10/2026 15:04", "2026-10-07"], {
        field: "x",
        to: "time",
        dateFormat: "DD/MM/YYYY HH:mm",
      }).values
    ).toEqual([local, null])
  })

  it("reads booleans from booleans, numbers and text", () => {
    expect(
      one([true, "true", "FALSE", 0, 3, "yes", null], {
        field: "x",
        to: "boolean",
      })
    ).toEqual({
      name: "x",
      type: "bool",
      values: [true, true, false, false, true, null, null],
    })
  })

  it("keeps values for enum and other but marks the field other", () => {
    for (const to of ["enum", "other"] as const) {
      expect(one(["a", 1], { field: "x", to })).toEqual({
        name: "x",
        type: "other",
        values: ["a", 1],
      })
    }
  })

  it("finds a field by its series name and applies conversions in order", () => {
    const frame = frameOf(
      { name: "a", type: "string", values: ["1"] },
      { name: "b", type: "string", seriesName: "B series", values: ["x"] }
    )
    const [out] = convertFieldType([frame], {
      type: "convertFieldType",
      conversions: [
        { field: "a", to: "number" },
        { field: "B series", to: "other" },
        { field: "a", to: "string" },
      ],
    })
    expect(out.fields.map((f) => [f.name, f.type, f.values])).toEqual([
      ["a", "string", ["1"]],
      ["b", "other", ["x"]],
    ])
  })

  it("does nothing for an unknown field and never changes its input", () => {
    const frame = Object.freeze(
      frameOf(
        Object.freeze({
          name: "x",
          type: "string" as const,
          values: Object.freeze(["1"]) as unknown as unknown[],
        })
      )
    )
    const [out] = convertFieldType([frame], {
      type: "convertFieldType",
      conversions: [{ field: "missing", to: "number" }],
    })
    expect(out).toEqual(frame)
    expect(frame.fields[0].type).toBe("string")
  })
})
