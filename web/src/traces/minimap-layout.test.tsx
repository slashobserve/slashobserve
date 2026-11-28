import { render, screen } from "@testing-library/react"
import { describe, expect, it, vi } from "vitest"

import { spanColumns } from "@/panels/traces/traces-panel"
import type { TraceViewProps } from "@/panels/traces/traces-panel"

import { TraceMinimap } from "./minimap.tsx"
import { wholeTrace } from "./minimap"

const model = { spans: [] } as unknown as TraceViewProps["model"]

describe("TraceMinimap layout", () => {
  it("sits in the same columns as the ruler so its origin is the ruler's", () => {
    render(<TraceMinimap model={model} zoom={wholeTrace} onZoom={vi.fn()} />)
    const strip = screen.getByRole("img", {
      name: "Overview of the whole trace",
    })
    const row = strip.parentElement?.parentElement
    const [wide] = spanColumns.split(" ")
    expect(row?.classList.contains(wide!)).toBe(true)
    expect(strip.parentElement?.classList.contains("px-2")).toBe(true)
  })

  it("puts Reset zoom in the duration column when zoomed", () => {
    render(
      <TraceMinimap
        model={model}
        zoom={{ start: 0.2, end: 0.4 }}
        onZoom={vi.fn()}
      />
    )
    const strip = screen.getByRole("img", {
      name: "Overview of the whole trace",
    })
    const row = strip.parentElement?.parentElement
    expect(row?.lastElementChild).toBe(
      screen.getByRole("button", { name: "Reset zoom" })
    )
  })
})
