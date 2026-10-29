import { describe, expect, it, vi } from "vitest"

import { createSharedCursor, noSharedCursor } from "./shared-cursor"

function twoMembers(mode: "crosshair" | "tooltip" | "off" = "crosshair") {
  const hub = createSharedCursor()
  hub.setMode(mode)
  const drawA = vi.fn()
  const drawB = vi.fn()
  const a = hub.join(drawA)
  const b = hub.join(drawB)
  return { hub, drawA, drawB, a, b }
}

describe("shared cursor hub", () => {
  it("never draws a peer while the mode is off", () => {
    const { drawA, drawB, a, b } = twoMembers("off")
    a.move(1000)
    b.move(2000)
    a.move(null)
    b.leave()
    expect(drawA).not.toHaveBeenCalled()
    expect(drawB).not.toHaveBeenCalled()
  })

  it("draws every other member at the owner's time and never the owner", () => {
    const { drawA, drawB, a } = twoMembers()
    a.move(1000)
    expect(drawB).toHaveBeenCalledExactlyOnceWith({ x: 1000, tooltip: false })
    expect(drawA).not.toHaveBeenCalled()
  })

  it("asks peers for a tooltip in tooltip mode", () => {
    const { drawB, a } = twoMembers("tooltip")
    a.move(1000)
    expect(drawB).toHaveBeenCalledWith({ x: 1000, tooltip: true })
  })

  it("moves the cursor to the member that was moved last", () => {
    const { drawA, drawB, a, b } = twoMembers()
    a.move(1000)
    b.move(2000)
    expect(drawA).toHaveBeenLastCalledWith({ x: 2000, tooltip: false })
    expect(drawB).toHaveBeenLastCalledWith({ x: 1000, tooltip: false })
    expect(drawB).toHaveBeenCalledTimes(1)
  })

  it("clears peers when the owner moves to null", () => {
    const { drawB, a } = twoMembers()
    a.move(1000)
    a.move(null)
    expect(drawB).toHaveBeenLastCalledWith(null)
  })

  it("ignores move(null) from a member that is not the owner", () => {
    const { drawA, drawB, a, b } = twoMembers()
    a.move(1000)
    drawA.mockClear()
    drawB.mockClear()
    b.move(null)
    expect(drawA).not.toHaveBeenCalled()
    expect(drawB).not.toHaveBeenCalled()
  })

  it("clears peers when the owner leaves", () => {
    const { drawB, a } = twoMembers()
    a.move(1000)
    a.leave()
    expect(drawB).toHaveBeenLastCalledWith(null)
  })

  it("stops drawing and moving a member that left, and ignores a second leave", () => {
    const { hub, drawA, drawB, a, b } = twoMembers()
    a.leave()
    drawA.mockClear()
    b.move(1000)
    a.move(5000)
    a.leave()
    expect(drawA).not.toHaveBeenCalled()
    expect(drawB).not.toHaveBeenCalled()
    hub.setMode("tooltip")
    expect(drawA).not.toHaveBeenCalled()
  })

  it("does not clear peers when a peer leaves", () => {
    const { drawA, a, b } = twoMembers()
    a.move(1000)
    drawA.mockClear()
    b.leave()
    expect(drawA).not.toHaveBeenCalled()
  })

  it("draws the current cursor on a member that joins later", () => {
    const { hub, a } = twoMembers()
    a.move(1000)
    const drawC = vi.fn()
    hub.join(drawC)
    expect(drawC).toHaveBeenCalledExactlyOnceWith({
      x: 1000,
      tooltip: false,
    })
  })

  it("does not draw a member that joins with no owner or with the mode off", () => {
    const hub = createSharedCursor()
    hub.setMode("crosshair")
    const early = vi.fn()
    hub.join(early)
    hub.setMode("off")
    const owner = hub.join(vi.fn())
    owner.move(1000)
    const late = vi.fn()
    hub.join(late)
    expect(early).toHaveBeenCalledExactlyOnceWith(null)
    expect(late).not.toHaveBeenCalled()
  })

  it("redraws non-owners when the mode changes and ignores the same mode", () => {
    const { hub, drawA, drawB, a } = twoMembers()
    a.move(1000)
    drawB.mockClear()
    hub.setMode("crosshair")
    expect(drawB).not.toHaveBeenCalled()
    hub.setMode("tooltip")
    expect(drawB).toHaveBeenLastCalledWith({ x: 1000, tooltip: true })
    hub.setMode("off")
    expect(drawB).toHaveBeenLastCalledWith(null)
    expect(drawA).not.toHaveBeenCalled()
  })

  it("redraws null on mode change when there is no owner", () => {
    const { hub, drawA, drawB } = twoMembers()
    hub.setMode("tooltip")
    expect(drawA).toHaveBeenLastCalledWith(null)
    expect(drawB).toHaveBeenLastCalledWith(null)
  })

  it("gives a no-op member when there is no hub", () => {
    const draw = vi.fn()
    const member = noSharedCursor.join(draw)
    member.move(1000)
    member.move(null)
    member.leave()
    expect(draw).not.toHaveBeenCalled()
  })
})
