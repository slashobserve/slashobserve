import { describe, expect, it } from "vitest"

import {
  boxMinHeight,
  boxMinWidth,
  boxStyle,
  clampBoxSize,
  draggedBoxSize,
  keyboardBoxSize,
  parseStoredBoxSizes,
} from "./canvas-box-size"

const room = { width: 1200, height: 800 }

describe("parseStoredBoxSizes", () => {
  it("reads the size kept for each box", () => {
    expect(
      parseStoredBoxSizes('{"ask":{"width":480,"height":520},"other":1}')
    ).toEqual({ ask: { width: 480, height: 520 } })
  })

  it.each([null, "", "nonsense", "[]", '{"ask":{"width":"480"}}', '{"ask":3}'])(
    "falls back to the default sizes for %s",
    (raw) => {
      expect(parseStoredBoxSizes(raw)).toEqual({})
    }
  )
})

describe("clampBoxSize", () => {
  it("keeps a box readable and inside the window", () => {
    expect(clampBoxSize({ width: 10, height: 10 }, room)).toEqual({
      width: boxMinWidth,
      height: boxMinHeight,
    })
    expect(clampBoxSize({ width: 5000, height: 5000 }, room)).toEqual(room)
  })

  it("prefers the smallest readable size on a window narrower than it", () => {
    expect(
      clampBoxSize({ width: 400, height: 400 }, { width: 200, height: 150 })
    ).toEqual({ width: boxMinWidth, height: boxMinHeight })
  })
})

describe("draggedBoxSize", () => {
  it("grows and shrinks from the size the drag started at", () => {
    const start = { width: 320, height: 300 }
    expect(draggedBoxSize(start, { x: 80, y: 40 }, room)).toEqual({
      width: 400,
      height: 340,
    })
    expect(draggedBoxSize(start, { x: -500, y: -500 }, room)).toEqual({
      width: boxMinWidth,
      height: boxMinHeight,
    })
  })
})

describe("keyboardBoxSize", () => {
  const size = { width: 320, height: 300 }

  it("resizes by a step on the arrow keys and a larger one with Shift", () => {
    expect(keyboardBoxSize(size, "ArrowRight", false, room)?.width).toBe(336)
    expect(keyboardBoxSize(size, "ArrowLeft", false, room)?.width).toBe(304)
    expect(keyboardBoxSize(size, "ArrowDown", true, room)?.height).toBe(364)
    expect(keyboardBoxSize(size, "ArrowUp", false, room)?.height).toBe(284)
  })

  it("leaves every other key alone", () => {
    expect(keyboardBoxSize(size, "Enter", false, room)).toBeNull()
  })
})

describe("boxStyle", () => {
  it("gives an untouched box its default width and its own height", () => {
    expect(boxStyle("comment", null)).toEqual({
      width: "min(320px, calc(100vw - 16px))",
    })
    expect(boxStyle("ask", null).width).toBe("min(384px, calc(100vw - 16px))")
  })

  it("never lets a resized box outgrow the window", () => {
    expect(boxStyle("ask", { width: 600, height: 500 })).toEqual({
      width: "min(600px, calc(100vw - 16px))",
      height: "min(500px, var(--available-height, 100vh))",
    })
  })
})
