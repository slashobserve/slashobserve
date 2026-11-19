import { describe, expect, it, vi } from "vitest"
import type uPlot from "uplot"

import { annotateGesture, type AnnotateRange } from "./gesture"

function plot() {
  const over = document.createElement("div")
  document.body.append(over)
  over.getBoundingClientRect = () => ({ left: 100 }) as DOMRect
  return {
    over,
    posToVal: (x: number) => 1_000 + x,
  } as unknown as uPlot
}

function mount(
  annotate: ((range: AnnotateRange) => void) | undefined = vi.fn()
) {
  const gesture = annotateGesture(() => annotate)
  const u = plot()
  ;(gesture.hooks!.init as (u: uPlot) => void)(u)
  return { gesture, u, annotate }
}

function press(
  u: uPlot,
  down: MouseEventInit,
  up: { clientX: number; clientY: number }
) {
  u.over.dispatchEvent(new MouseEvent("mousedown", { bubbles: true, ...down }))
  document.dispatchEvent(new MouseEvent("mouseup", { bubbles: true, ...up }))
}

describe("annotateGesture", () => {
  it("annotates the clicked time when Cmd is held", () => {
    const { u, annotate } = mount()
    press(
      u,
      { metaKey: true, clientX: 150, clientY: 40 },
      { clientX: 151, clientY: 40 }
    )
    expect(annotate).toHaveBeenCalledWith({ fromMs: 1_050_000 })
  })

  it("accepts Ctrl as well", () => {
    const { u, annotate } = mount()
    press(
      u,
      { ctrlKey: true, clientX: 150, clientY: 40 },
      { clientX: 150, clientY: 40 }
    )
    expect(annotate).toHaveBeenCalledTimes(1)
  })

  it("leaves a plain click alone", () => {
    const { u, annotate, gesture } = mount()
    press(u, { clientX: 150, clientY: 40 }, { clientX: 150, clientY: 40 })
    expect(annotate).not.toHaveBeenCalled()
    expect(gesture.consumeSelect()).toBe(false)
  })

  it("claims the selection of a Cmd drag once", () => {
    const { u, annotate, gesture } = mount()
    press(
      u,
      { metaKey: true, clientX: 150, clientY: 40 },
      { clientX: 250, clientY: 40 }
    )
    expect(annotate).not.toHaveBeenCalled()
    expect(gesture.consumeSelect()).toBe(true)
    expect(gesture.consumeSelect()).toBe(false)
  })

  it("does nothing when the viewer cannot annotate", () => {
    const gesture = annotateGesture(() => undefined)
    const u = plot()
    ;(gesture.hooks!.init as (u: uPlot) => void)(u)
    press(
      u,
      { metaKey: true, clientX: 150, clientY: 40 },
      { clientX: 250, clientY: 40 }
    )
    expect(gesture.consumeSelect()).toBe(false)
  })
})
