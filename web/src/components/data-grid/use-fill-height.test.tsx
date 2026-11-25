import { act, render } from "@testing-library/react"
import { useRef } from "react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { useFillHeight } from "./use-fill-height"

const tops = new Map<string, number>()
const resized: ResizeObserverCallback[] = []

function Box() {
  const ref = useRef<HTMLDivElement>(null)
  const height = useFillHeight(ref, true)
  return <div ref={ref} data-testid="box" data-height={height} />
}

function renderInScroller(scrollTop: number) {
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(
    function (this: HTMLElement) {
      return { top: tops.get(this.dataset.testid ?? "") ?? 0 } as DOMRect
    }
  )
  vi.spyOn(HTMLElement.prototype, "clientHeight", "get").mockImplementation(
    function (this: HTMLElement) {
      return this.dataset.testid === "scroller" ? 800 : 0
    }
  )
  vi.spyOn(HTMLElement.prototype, "scrollTop", "get").mockImplementation(
    function (this: HTMLElement) {
      return this.dataset.testid === "scroller" ? scrollTop : 0
    }
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
  return render(
    <div data-testid="scroller" style={{ overflowY: "auto" }}>
      <section>
        <Box />
      </section>
    </div>
  )
}

afterEach(() => {
  tops.clear()
  resized.length = 0
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe("useFillHeight", () => {
  it("fills the scroll area below the box even when the area is scrolled", () => {
    tops.set("scroller", 100)
    tops.set("box", -500)
    const { getByTestId } = renderInScroller(900)
    expect(getByTestId("box")).toHaveAttribute("data-height", "452")
  })

  it("measures again when the content above the box changes size", () => {
    tops.set("scroller", 100)
    tops.set("box", 200)
    const { getByTestId } = renderInScroller(0)
    expect(getByTestId("box")).toHaveAttribute("data-height", "652")
    tops.set("box", 400)
    act(() => {
      for (const callback of resized) {
        callback([], {} as ResizeObserver)
      }
    })
    expect(getByTestId("box")).toHaveAttribute("data-height", "452")
  })
})
