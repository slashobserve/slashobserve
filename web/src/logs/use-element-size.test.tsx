import { render, screen } from "@testing-library/react"
import { afterEach, beforeEach, expect, it, vi } from "vitest"

import { useElementSize } from "./use-element-size"

class SilentObserver {
  observe() {}
  disconnect() {}
}

function Probe() {
  const { ref, width, height } = useElementSize<HTMLDivElement>()
  return (
    <div ref={ref} data-testid="probe">
      {width}x{height}
    </div>
  )
}

beforeEach(() => {
  vi.stubGlobal("ResizeObserver", SilentObserver)
})

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

it("measures the element on mount when the observer never reports", () => {
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({
    width: 640.7,
    height: 480.2,
  } as DOMRect)
  render(<Probe />)
  expect(screen.getByTestId("probe").textContent).toBe("640x480")
})
