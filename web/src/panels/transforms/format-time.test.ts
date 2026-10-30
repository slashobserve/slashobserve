import { describe, expect, it } from "vitest"

import { formatTime } from "./format-time"
import type { PreparedFrame } from "./names"

const at = Date.UTC(2026, 9, 7, 15, 4, 9)

const frame: PreparedFrame = {
  ref: "A",
  fields: [
    { name: "Time", type: "time", values: [at, null] },
    { name: "Value", type: "number", values: [1, 2] },
  ],
}

describe("formatTime", () => {
  it("turns a time field into a string field in UTC when asked", () => {
    const [out] = formatTime([frame], {
      type: "formatTime",
      field: "Time",
      format: "YYYY-MM-DD HH:mm",
      utc: true,
    })
    expect(out.fields[0]).toEqual({
      name: "Time",
      type: "string",
      values: ["2026-10-07 15:04", null],
    })
    expect(out.fields[1]).toBe(frame.fields[1])
  })

  it("writes local time without utc", () => {
    const local = new Date(2026, 9, 7, 15, 4).getTime()
    const [out] = formatTime(
      [{ ref: "A", fields: [{ name: "Time", type: "time", values: [local] }] }],
      { type: "formatTime", field: "Time", format: "HH:mm" }
    )
    expect(out.fields[0].values).toEqual(["15:04"])
  })

  it("leaves a field that is not a time field or not named", () => {
    for (const field of ["Value", "missing"]) {
      const [out] = formatTime([frame], {
        type: "formatTime",
        field,
        format: "YYYY",
      })
      expect(out).toEqual(frame)
    }
  })
})
