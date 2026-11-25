import { useRef } from "react"
import { act, render, screen } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { besideDetailMinWidth, useBesideDetail } from "./use-beside-detail"

let resize: () => void = () => undefined
let shellWidth = 0

class FakeObserver {
  constructor(callback: () => void) {
    resize = callback
  }
  observe() {}
  disconnect() {}
}

vi.stubGlobal("ResizeObserver", FakeObserver)

afterEach(() => {
  shellWidth = 0
})

function Beside() {
  const ref = useRef<HTMLDivElement>(null)
  return (
    <div ref={ref}>
      <output>{String(useBesideDetail(ref))}</output>
    </div>
  )
}

function Probe() {
  return (
    <div className="@container" data-testid="shell">
      <Beside />
    </div>
  )
}

describe("useBesideDetail", () => {
  it("follows the shell container's width across the breakpoint", () => {
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(
      function (this: HTMLElement) {
        return {
          width: this.dataset.testid === "shell" ? shellWidth : 0,
        } as DOMRect
      }
    )
    shellWidth = besideDetailMinWidth - 1
    render(<Probe />)
    expect(screen.getByRole("status")).toHaveTextContent("false")
    shellWidth = besideDetailMinWidth
    act(() => resize())
    expect(screen.getByRole("status")).toHaveTextContent("true")
  })
})
