import { describe, expect, it } from "vitest"

import type { Anchor, AnchorsModule, Point, Rect } from "@/panels/anchors"
import * as timeseries from "@/panels/timeseries/anchors"
import type { TimeseriesAnchorLayout } from "@/panels/timeseries/anchors"

import {
  dragAnchor,
  minDrag,
  outlineOf,
  pinView,
  placePin,
  pointerAnchor,
  revealAnchor,
  titleAnchor,
  withWindow,
  type CanvasSurface,
} from "./canvas-target"

const body = { width: 400, height: 200 }

const bar: Rect = { x: 100, y: 40, width: 40, height: 120 }

const module: AnchorsModule = {
  hitTest: ((layout: { hit: boolean }, point: Point) =>
    layout.hit
      ? {
          kind: "bar",
          pos: { x: point.x / 400, y: point.y / 200 },
          category: "checkout",
        }
      : null) as AnchorsModule["hitTest"],
  locate: ((layout: { bar: Rect | null }, anchor: Anchor) =>
    anchor.category === "checkout" ? layout.bar : null) as NonNullable<
    AnchorsModule["locate"]
  >,
}

function surface(layout: { hit?: boolean; bar?: Rect | null }): CanvasSurface {
  return {
    module,
    published: { type: "barchart", layout: { hit: true, bar, ...layout } },
  }
}

const blank: CanvasSurface = { module: null, published: null }

const onBar: Anchor = {
  kind: "bar",
  pos: { x: 0.3, y: 0.1 },
  category: "checkout",
}

describe("pointerAnchor", () => {
  it("uses the panel's hit test", () => {
    expect(pointerAnchor(surface({}), { x: 120, y: 100 }, body)).toMatchObject({
      kind: "bar",
      category: "checkout",
    })
  })

  it("falls back to the spot when nothing is under the pointer", () => {
    expect(
      pointerAnchor(surface({ hit: false }), { x: 100, y: 50 }, body)
    ).toEqual({ kind: "position", pos: { x: 0.25, y: 0.25 } })
    expect(pointerAnchor(blank, { x: 100, y: 50 }, body)).toEqual({
      kind: "position",
      pos: { x: 0.25, y: 0.25 },
    })
  })

  it("pins a title anchor to the top edge", () => {
    expect(titleAnchor({ x: 200, y: -20 }, body)).toEqual({
      kind: "title",
      pos: { x: 0.5, y: 0 },
    })
  })
})

describe("outlineOf", () => {
  it("outlines the target the panel can locate", () => {
    expect(outlineOf(surface({}), onBar)).toEqual(bar)
  })

  it("outlines the whole panel otherwise", () => {
    expect(outlineOf(surface({}), null)).toBeNull()
    expect(
      outlineOf(surface({}), { kind: "position", pos: { x: 0, y: 0 } })
    ).toBeNull()
    expect(outlineOf(blank, onBar)).toBeNull()
  })
})

describe("placePin", () => {
  it("keeps the stored spot when it is inside the located target", () => {
    expect(
      placePin({ ...onBar, pos: { x: 0.3, y: 0.5 } }, body, surface({}), "data")
    ).toEqual({ at: { x: 120, y: 100 }, gone: false })
  })

  it("moves the pin onto the target when the data moved", () => {
    expect(placePin(onBar, body, surface({}), "data")).toEqual({
      at: { x: 120, y: 40 },
      gone: false,
    })
  })

  it("falls back to the stored spot when the data is gone", () => {
    expect(
      placePin({ ...onBar, category: "search" }, body, surface({}), "data")
    ).toEqual({ at: { x: 120, y: 20 }, gone: true })
    expect(placePin(onBar, body, surface({}), "empty")).toEqual({
      at: { x: 120, y: 20 },
      gone: true,
    })
  })

  it("trusts the stored spot when the panel cannot locate anchors", () => {
    expect(placePin(onBar, body, blank, "data")).toEqual({
      at: { x: 120, y: 20 },
      gone: false,
    })
  })

  it("never calls a spot or a title gone", () => {
    for (const kind of ["position", "title"] as const) {
      expect(
        placePin({ kind, pos: { x: 0.5, y: 0.5 } }, body, surface({}), "empty")
      ).toEqual({ at: { x: 200, y: 100 }, gone: false })
    }
  })
})

describe("placePin on a time chart", () => {
  const chart: TimeseriesAnchorLayout = {
    body,
    plot: { x: 0, y: 0, width: 400, height: 200 },
    xMin: 1000,
    xMax: 1400,
    xs: [1000, 1100, 1200, 1300, 1400],
    series: [
      {
        name: "checkout",
        labels: { service: "checkout" },
        ys: [100, 100, 100, 100, 100],
        values: [1, 1, 1, 1, 1],
        format: String,
      },
    ],
    hidden: [{ service: "payments" }],
  }
  const onChart: CanvasSurface = {
    module: timeseries as AnchorsModule,
    published: { type: "timeseries", layout: chart },
  }
  const iso = (seconds: number) => new Date(seconds * 1000).toISOString()
  const pos = { x: 0.5, y: 0.5 }

  it("draws no pin for a moment outside the shown range, rather than calling it gone", () => {
    for (const anchor of [
      { kind: "time", pos, from: iso(500) },
      { kind: "point", pos, from: iso(2000), series: { service: "checkout" } },
      { kind: "range", pos, from: iso(100), to: iso(900) },
    ] as Anchor[]) {
      expect(placePin(anchor, body, onChart, "data")).toBeNull()
    }
  })

  it("keeps a range that is partly in view on the plot", () => {
    expect(
      placePin(
        {
          kind: "range",
          pos: { x: 0.9, y: 0.5 },
          from: iso(500),
          to: iso(1100),
        },
        body,
        onChart,
        "data"
      )
    ).toEqual({ at: { x: 101, y: 100 }, gone: false })
  })

  it("calls a point gone only when its series no longer exists while its time is in view", () => {
    expect(
      placePin(
        { kind: "point", pos, from: iso(1200), series: { service: "search" } },
        body,
        onChart,
        "data"
      )
    ).toEqual({ at: { x: 200, y: 100 }, gone: true })
  })

  it("draws no pin on a series hidden from the legend", () => {
    for (const anchor of [
      { kind: "point", pos, from: iso(1200), series: { service: "payments" } },
      { kind: "series", pos, series: { service: "payments" } },
    ] as Anchor[]) {
      expect(placePin(anchor, body, onChart, "data")).toBeNull()
    }
  })
})

describe("placePin while the panel shows something other than its chart", () => {
  it("draws only title pins in the error state, while loading and in the table view", () => {
    expect(placePin(onBar, body, surface({}), "hidden")).toBeNull()
    expect(
      placePin(
        { kind: "position", pos: { x: 0.5, y: 0.5 } },
        body,
        blank,
        "hidden"
      )
    ).toBeNull()
    expect(
      placePin({ kind: "title", pos: { x: 0.5, y: 0 } }, body, blank, "hidden")
    ).toEqual({ at: { x: 200, y: 0 }, gone: false })
  })
})

describe("pinView", () => {
  it("shows data pins only while the panel draws its own data", () => {
    expect(pinView("ready", false)).toBe("data")
    expect(pinView("empty", false)).toBe("empty")
    for (const state of ["error", "loading", "unsupported"] as const) {
      expect(pinView(state, false)).toBe("hidden")
    }
    expect(pinView("ready", true)).toBe("hidden")
  })
})

describe("withWindow", () => {
  it("records the time range the panel showed", () => {
    expect(withWindow(onBar, { from: "now-6h", to: "now" })).toEqual({
      ...onBar,
      window: { from: "now-6h", to: "now" },
    })
  })

  it("leaves the anchor alone without a full range", () => {
    expect(withWindow(onBar, { from: "", to: "now" })).toBe(onBar)
  })
})

describe("dragAnchor", () => {
  const ranged: CanvasSurface = {
    module: {
      ...module,
      rangeTest: ((_layout: unknown, start: Point, end: Point) => ({
        kind: "range",
        pos: { x: 0, y: 0 },
        from: String(start.x),
        to: String(end.x),
      })) as NonNullable<AnchorsModule["rangeTest"]>,
    },
    published: { type: "timeseries", layout: {} },
  }

  it("names a range when the pointer moved across the axis", () => {
    expect(
      dragAnchor(ranged, { x: 10, y: 50 }, { x: 10 + minDrag, y: 90 })
    ).toMatchObject({ kind: "range", from: "10", to: String(10 + minDrag) })
  })

  it("treats a short move as a click", () => {
    expect(
      dragAnchor(ranged, { x: 10, y: 50 }, { x: 10 + minDrag - 1, y: 120 })
    ).toBeNull()
  })

  it("needs a panel that can name a range", () => {
    expect(
      dragAnchor(surface({}), { x: 10, y: 50 }, { x: 90, y: 50 })
    ).toBeNull()
    expect(dragAnchor(blank, { x: 10, y: 50 }, { x: 90, y: 50 })).toBeNull()
  })
})

describe("revealAnchor", () => {
  it("asks the panel to bring the anchor's data into view", () => {
    const asked: Anchor[] = []
    const revealing: CanvasSurface = {
      module: {
        ...module,
        reveal: ((_layout: unknown, anchor: Anchor) => {
          asked.push(anchor)
          return true
        }) as NonNullable<AnchorsModule["reveal"]>,
      },
      published: { type: "table", layout: {} },
    }
    expect(revealAnchor(revealing, onBar)).toBe(true)
    expect(asked).toEqual([onBar])
  })

  it("does nothing for a panel that cannot reveal", () => {
    expect(revealAnchor(surface({}), onBar)).toBe(false)
    expect(revealAnchor(blank, onBar)).toBe(false)
  })
})
