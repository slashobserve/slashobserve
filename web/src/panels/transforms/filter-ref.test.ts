import { describe, expect, it } from "vitest"

import { filterByRefId } from "./filter-ref"
import type { PreparedFrame } from "./names"

const frames: PreparedFrame[] = [
  { ref: "A", fields: [{ name: "a", type: "number", values: [1] }] },
  { ref: "B", fields: [{ name: "b", type: "number", values: [2] }] },
  { ref: "C", fields: [{ name: "c", type: "number", values: [4] }] },
]

describe("filterByRefId", () => {
  it("keeps the frames of the listed queries in their order", () => {
    const out = filterByRefId(frames, {
      type: "filterByRefId",
      include: ["C", "A"],
    })
    expect(out.map((frame) => frame.ref)).toEqual(["A", "C"])
  })

  it("gives nothing for unknown refs", () => {
    expect(
      filterByRefId(frames, { type: "filterByRefId", include: ["Z"] })
    ).toEqual([])
  })
})
