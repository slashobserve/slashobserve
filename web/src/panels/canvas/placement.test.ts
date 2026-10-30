import { describe, expect, it } from "vitest"

import { clampView, elementBox, parsePlacement, zoomedView } from "./placement"

const body = { width: 400, height: 200 }
const percent = { left: 10, top: 20, width: 25, height: 50 }

function place(fields: Record<string, unknown>) {
  const placement = parsePlacement(fields)
  if (!placement) throw new Error("placement did not parse")
  return elementBox(percent, placement, body)
}

describe("elementBox", () => {
  it("uses the percent box of the body without a placement", () => {
    expect(elementBox(percent, null, body)).toEqual({
      left: 40,
      top: 40,
      width: 100,
      height: 100,
    })
  })

  it.each([
    [{ horizontal: "left", left: 30, width: 80 }, 30, 80],
    [{ horizontal: "right", right: 20, width: 80 }, 300, 80],
    [{ horizontal: "leftright", left: 30, right: 50 }, 30, 320],
    [{ horizontal: "center", left: 0, width: 100 }, 150, 100],
    [{ horizontal: "center", left: 20, width: 100 }, 170, 100],
    [{ horizontal: "center", left: -60, width: 40 }, 120, 40],
    [{ horizontal: "scale", left: 10, width: 50 }, 40, 200],
  ])("places %j horizontally", (fields, left, width) => {
    const box = place({ vertical: "top", ...fields })
    expect([box.left, box.width]).toEqual([left, width])
  })

  it.each([
    [{ vertical: "top", top: 10, height: 60 }, 10, 60],
    [{ vertical: "bottom", bottom: 10, height: 60 }, 130, 60],
    [{ vertical: "topbottom", top: 20, bottom: 30 }, 20, 150],
    [{ vertical: "center", top: -10, height: 50 }, 65, 50],
    [{ vertical: "scale", top: 50, height: 25 }, 100, 50],
  ])("places %j vertically", (fields, top, height) => {
    const box = place({ horizontal: "left", ...fields })
    expect([box.top, box.height]).toEqual([top, height])
  })

  it("takes a missing number from the percent box so the element stays where it was", () => {
    for (const horizontal of [
      "left",
      "right",
      "leftright",
      "center",
      "scale",
    ]) {
      expect(place({ horizontal, vertical: "top" })).toMatchObject({
        left: 40,
        width: 100,
      })
    }
    for (const vertical of ["top", "bottom", "topbottom", "center", "scale"]) {
      expect(place({ horizontal: "left", vertical })).toMatchObject({
        top: 40,
        height: 100,
      })
    }
  })

  it("never returns a negative size", () => {
    const box = place({
      horizontal: "leftright",
      vertical: "topbottom",
      left: 300,
      right: 300,
      top: 150,
      bottom: 150,
    })
    expect([box.width, box.height]).toEqual([0, 0])
  })
})

describe("parsePlacement", () => {
  it.each([
    null,
    "left",
    [],
    {},
    { horizontal: "left" },
    { horizontal: "top", vertical: "top" },
    { horizontal: "left", vertical: "left" },
    { horizontal: "constructor", vertical: "top" },
  ])("rejects %j", (raw) => {
    expect(parsePlacement(raw)).toBeNull()
  })

  it("ignores numbers that are not finite", () => {
    const placement = parsePlacement({
      horizontal: "left",
      vertical: "top",
      left: "5",
      top: Number.NaN,
      width: 10,
    })
    expect(placement?.x).toEqual({
      mode: "start",
      start: null,
      end: null,
      size: 10,
    })
    expect(placement?.y.start).toBeNull()
  })
})

describe("clampView", () => {
  const view = { zoom: 2, panX: 500, panY: -500 }

  it("keeps the content over the body", () => {
    expect(clampView(view, body, false)).toEqual({
      zoom: 2,
      panX: 200,
      panY: -100,
    })
  })

  it("does not pan when the content is not larger than the body", () => {
    expect(clampView({ zoom: 1, panX: 30, panY: 30 }, body, false)).toEqual({
      zoom: 1,
      panX: 0,
      panY: 0,
    })
    expect(clampView({ zoom: 0.5, panX: 30, panY: 30 }, body, false)).toEqual({
      zoom: 0.5,
      panX: 0,
      panY: 0,
    })
  })

  it("lifts the bounds with infinite pan", () => {
    expect(clampView(view, body, true)).toEqual(view)
  })
})

describe("zoomedView", () => {
  it("zooms about the center and clamps the pan", () => {
    const zoomedOut = zoomedView(
      { zoom: 2, panX: 200, panY: 0 },
      0.5,
      body,
      false
    )
    expect(zoomedOut).toEqual({ zoom: 1, panX: 0, panY: 0 })
  })

  it("stops at the zoom limits", () => {
    const view = zoomedView({ zoom: 4, panX: 0, panY: 0 }, 2, body, false)
    expect(view.zoom).toBe(4)
  })
})
