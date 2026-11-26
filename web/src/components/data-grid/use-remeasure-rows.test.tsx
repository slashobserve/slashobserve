import { render } from "@testing-library/react"
import { useRef } from "react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { useRemeasureRows } from "./use-remeasure-rows"

const resized: ResizeObserverCallback[] = []
let width = 800

function Box({ api, active }: { api: object; active: boolean }) {
  const ref = useRef<HTMLDivElement>(null)
  const apiRef = useRef(api as never)
  useRemeasureRows(ref, apiRef, active)
  return <div ref={ref} />
}

function resize() {
  for (const callback of resized) callback([], {} as ResizeObserver)
}

beforeEach(() => {
  width = 800
  resized.length = 0
  vi.useFakeTimers()
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(
    () => ({ width }) as DOMRect
  )
  vi.stubGlobal(
    "ResizeObserver",
    class {
      constructor(callback: ResizeObserverCallback) {
        resized.push(callback)
      }
      observe() {}
      disconnect() {}
    }
  )
})

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

describe("useRemeasureRows", () => {
  it("resets the row heights once when the width changes", () => {
    const api = { resetRowHeights: vi.fn(), isDestroyed: () => false }
    render(<Box api={api} active />)
    width = 500
    resize()
    resize()
    vi.advanceTimersByTime(200)
    expect(api.resetRowHeights).toHaveBeenCalledTimes(1)
  })

  it("waits until the resizing stops", () => {
    const api = { resetRowHeights: vi.fn(), isDestroyed: () => false }
    render(<Box api={api} active />)
    width = 500
    resize()
    vi.advanceTimersByTime(100)
    width = 450
    resize()
    vi.advanceTimersByTime(100)
    expect(api.resetRowHeights).not.toHaveBeenCalled()
    vi.advanceTimersByTime(60)
    expect(api.resetRowHeights).toHaveBeenCalledTimes(1)
  })

  it("ignores a resize that keeps the width", () => {
    const api = { resetRowHeights: vi.fn(), isDestroyed: () => false }
    render(<Box api={api} active />)
    resize()
    vi.advanceTimersByTime(200)
    expect(api.resetRowHeights).not.toHaveBeenCalled()
  })

  it("does nothing while inactive", () => {
    const api = { resetRowHeights: vi.fn(), isDestroyed: () => false }
    render(<Box api={api} active={false} />)
    expect(resized).toHaveLength(0)
  })

  it("skips a destroyed grid", () => {
    const api = { resetRowHeights: vi.fn(), isDestroyed: () => true }
    render(<Box api={api} active />)
    width = 500
    resize()
    vi.advanceTimersByTime(200)
    expect(api.resetRowHeights).not.toHaveBeenCalled()
  })
})
