import { describe, expect, it } from "vitest"

import { rendererManifests } from "./index"
import { clip, positionOf, seriesLabels } from "./anchors"

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
})
