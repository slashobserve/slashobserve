import { describe, expect, it } from "vitest"

import type { Span } from "@/panels/traces/trace-model"

import {
  isWholeTrace,
  minimapRows,
  zoomedOffset,
  zoomFromDrag,
} from "./minimap"

function span(offset: number, width: number, extra: Partial<Span> = {}): Span {
  return {
    id: `${offset}:${width}`,
    parentId: null,
    service: "web",
    operation: "op",
    start: 0,
    duration: 0,
    offset,
    width,
    depth: 0,
    slot: 0,
    error: false,
    hasChildren: false,
    attributes: [],
    kind: null,
    events: [],
    links: [],
    ...extra,
  }
}

describe("minimapRows", () => {
  it("gives 1000 spans in a 34 px strip 1 px lanes that overdraw", () => {
    const spans = Array.from({ length: 1000 }, (_, index) =>
      span(index / 1000, 0.001)
    )
    const rows = minimapRows(spans, 34)
    expect(rows).toHaveLength(1000)
    expect(rows[1].top - rows[0].top).toBe(1)
    expect(rows[999].top).toBe(999)
  })

  it("spreads a few spans over the strip and keeps colour and error", () => {
    const rows = minimapRows(
      [span(0, 1, { slot: 2 }), span(0.25, 0.5, { error: true })],
      34
    )
    expect(rows).toEqual([
      { top: 0, left: 0, width: 1, slot: 2, error: false },
      { top: 17, left: 0.25, width: 0.5, slot: 0, error: true },
    ])
  })

  it("is empty without spans", () => {
    expect(minimapRows([], 34)).toEqual([])
  })
})

describe("zoomFromDrag", () => {
  const whole = { start: 0, end: 1 }

  it("maps a drag across the strip onto the whole trace in either direction", () => {
    expect(zoomFromDrag(100, 300, 400, whole)).toEqual({
      start: 0.25,
      end: 0.75,
    })
    expect(zoomFromDrag(300, 100, 400, whole)).toEqual({
      start: 0.25,
      end: 0.75,
    })
  })

  it("narrows further inside a half zoom", () => {
    expect(zoomFromDrag(0, 200, 400, { start: 0.5, end: 1 })).toEqual({
      start: 0.5,
      end: 0.75,
    })
  })

  it("clips a drag that leaves the strip", () => {
    expect(zoomFromDrag(200, 900, 400, whole)).toEqual({ start: 0.5, end: 1 })
  })

  it.each([
    ["a click", 100, 100],
    ["a 1 px move", 100, 101],
  ])("treats %s as no zoom", (_, anchor, x) => {
    expect(zoomFromDrag(anchor, x, 400, whole)).toBeNull()
  })

  it("returns null for a strip without width", () => {
    expect(zoomFromDrag(0, 100, 0, whole)).toBeNull()
  })
})

describe("zoomedOffset", () => {
  const zoom = { start: 0.25, end: 0.75 }

  it("rescales a span inside the window", () => {
    expect(zoomedOffset({ offset: 0.5, width: 0.125 }, zoom)).toEqual({
      offset: 0.5,
      width: 0.25,
    })
  })

  it("clips a span crossing the window edge", () => {
    expect(zoomedOffset({ offset: 0, width: 0.5 }, zoom)).toEqual({
      offset: 0,
      width: 0.5,
    })
    expect(zoomedOffset({ offset: 0.625, width: 0.375 }, zoom)).toEqual({
      offset: 0.75,
      width: 0.25,
    })
  })

  it("returns null for a span outside the window", () => {
    expect(zoomedOffset({ offset: 0.8, width: 0.1 }, zoom)).toBeNull()
    expect(zoomedOffset({ offset: 0, width: 0.1 }, zoom)).toBeNull()
  })

  it("leaves a span alone over the whole trace", () => {
    expect(
      zoomedOffset({ offset: 0.3, width: 0.2 }, { start: 0, end: 1 })
    ).toEqual({ offset: 0.3, width: 0.2 })
  })
})

describe("isWholeTrace", () => {
  it("is true only for the full window", () => {
    expect(isWholeTrace({ start: 0, end: 1 })).toBe(true)
    expect(isWholeTrace({ start: 0.1, end: 1 })).toBe(false)
  })
})
