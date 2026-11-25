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
    <div
      className="@container"
      data-testid="shell"
      style={{ paddingLeft: 24, paddingRight: 24 }}
    >
      <Beside />
    </div>
  )
}

describe("useBesideDetail", () => {
  it("compares the shell's content width, without its padding, to the breakpoint", () => {
    vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockImplementation(
      function (this: HTMLElement) {
        return this.dataset.testid === "shell" ? shellWidth : 0
      }
    )
    shellWidth = besideDetailMinWidth + 47
    render(<Probe />)
    expect(screen.getByRole("status")).toHaveTextContent("false")
    shellWidth = besideDetailMinWidth + 48
    act(() => resize())
    expect(screen.getByRole("status")).toHaveTextContent("true")
    shellWidth = besideDetailMinWidth + 47
    act(() => resize())
    expect(screen.getByRole("status")).toHaveTextContent("false")
  })
})
