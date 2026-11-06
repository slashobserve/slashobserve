import { render, screen, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it } from "vitest"

import type { Step } from "@/observe/request-model"

import { StepTimeline } from "./step-timeline"

function step(overrides: Partial<Step>): Step {
  return {
    at: "2026-01-01T00:00:00Z",
    attempt: 0,
    phase: "discover",
    tool: "list_metrics",
    status: "ok",
    message: "Listed 37 metrics on prom",
    duration_ms: 10,
    ...overrides,
  }
}

const steps = [
  step({ message: "Listed 37 metrics on prom" }),
  step({ message: "Listed 12 metrics on prom" }),
  step({
    phase: "save",
    tool: "update_panels",
    message: "Saved panel 1 of 2: p95 latency",
  }),
]

describe("StepTimeline", () => {
  it("renders one item per phase run with its label and tool counts", () => {
    render(<StepTimeline steps={steps} working={false} reconnecting={false} />)
    const items = within(screen.getByRole("list", { name: "Progress" }))
      .getAllByRole("listitem")
      .filter((item) => item.parentElement?.tagName === "OL")
    expect(items).toHaveLength(2)
    expect(items[0]).toHaveTextContent(/^Discovering/)
    expect(items[0]).toHaveTextContent("list_metrics 2")
  })

  it("reveals the messages of a phase run on Show", async () => {
    const user = userEvent.setup()
    render(<StepTimeline steps={steps} working={false} reconnecting={false} />)
    expect(screen.queryByText("Listed 37 metrics on prom")).toBeNull()
    const toggle = screen.getByRole("button", { name: "Show 2 steps" })
    expect(toggle).toHaveAttribute("aria-expanded", "false")
    await user.click(toggle)
    expect(screen.getByText("Listed 37 metrics on prom")).toBeInTheDocument()
    expect(screen.getByText("Listed 12 metrics on prom")).toBeInTheDocument()
    expect(
      screen.getByRole("button", { name: "Hide 2 steps" })
    ).toHaveAttribute("aria-expanded", "true")
  })

  it("shows a save message without a button and the working row", () => {
    render(<StepTimeline steps={steps} working reconnecting={false} />)
    expect(
      screen.getByText("Saved panel 1 of 2: p95 latency")
    ).toBeInTheDocument()
    expect(screen.getAllByRole("button")).toHaveLength(1)
    expect(screen.getByText("Working")).toBeInTheDocument()
  })
})
