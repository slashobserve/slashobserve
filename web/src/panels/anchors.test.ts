import { describe, expect, it } from "vitest"

import { rendererManifests } from "./index"
import {
  clip,
  fit,
  labelsOf,
  msToIso,
  positionOf,
  seriesLabels,
} from "./anchors"

describe("anchor manifests", () => {
  const withAnchors = [...rendererManifests.values()].filter(
    (manifest) => manifest.anchors !== undefined
  )

  it("declares anchors for the first four types", () => {
    const types = withAnchors.map((manifest) => manifest.type)
    expect(types).toEqual(
      expect.arrayContaining(["timeseries", "table", "stat", "barchart"])
    )
  })

  it.each(withAnchors.map((manifest) => [manifest.type, manifest] as const))(
    "%s loads a hitTest",
    async (_type, manifest) => {
      const loaded = await manifest.anchors?.()
      expect(typeof loaded?.hitTest).toBe("function")
    }
  )

  it("leaves a type with no target to a comment on the whole panel", () => {
    expect(rendererManifests.get("placeholder")?.anchors).toBeUndefined()
  })
})

describe("anchor helpers", () => {
  it("keeps the position inside the panel body", () => {
    expect(positionOf({ x: -5, y: 250 }, { width: 100, height: 200 })).toEqual({
      x: 0,
      y: 1,
    })
    expect(positionOf({ x: 10, y: 10 }, { width: 0, height: 0 })).toEqual({
      x: 0,
      y: 0,
    })
  })

  it("falls back to the series name when it has no labels", () => {
    expect(seriesLabels(undefined, "up")).toEqual({ name: "up" })
    expect(seriesLabels({}, "up")).toEqual({ name: "up" })
    expect(seriesLabels({ job: "api" }, "up")).toEqual({ job: "api" })
  })

  it("clips text to the length the server accepts", () => {
    expect(clip("a".repeat(300))).toHaveLength(200)
  })

  it("keeps at most 16 labels and drops an empty name", () => {
    const entries = Array.from(
      { length: 20 },
      (_, n) => [`k${n}`, "v"] as const
    )
    expect(Object.keys(labelsOf([["", "x"], ...entries]))).toHaveLength(16)
    expect(labelsOf([["", "x"]])).toEqual({})
  })

  it("returns no time for a value that is not a date", () => {
    expect(msToIso(Number.NaN)).toBeNull()
    expect(msToIso(Number.POSITIVE_INFINITY)).toBeNull()
    expect(msToIso(0)).toBe("1970-01-01T00:00:00.000Z")
  })

  it("drops trailing labels until the anchor fits, and gives up on a lone oversize value", () => {
    const labels = Object.fromEntries(
      Array.from({ length: 16 }, (_, n) => [
        `${n}-${"k".repeat(190)}`,
        "v".repeat(200),
      ])
    )
    const fitted = fit({
      kind: "series",
      pos: { x: 0, y: 0 },
      series: labels,
    })
    expect(Object.keys(fitted?.series ?? {}).length).toBeLessThan(16)
    expect(Object.keys(fitted?.series ?? {}).length).toBeGreaterThan(0)
    expect(
      fit({
        kind: "series",
        pos: { x: 0, y: 0 },
        series: { a: "<".repeat(200) },
        quote: "<".repeat(200),
        shown: "<".repeat(200),
        category: "<".repeat(200),
      })
    ).toBeNull()
  })
})
