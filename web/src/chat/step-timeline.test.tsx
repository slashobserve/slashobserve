import { render, screen, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, describe, expect, it, vi } from "vitest"

import type { Step } from "@/observe/request-model"
import { renderWithProviders } from "@/test/render"

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

const requestId = "a77cc709-1b2c-4d3e-8f4a-5b6c7d8e9f01"

const steps = [
  step({ message: "Listed 37 metrics on prom" }),
  step({ message: "Listed 12 metrics on prom" }),
  step({
    phase: "save",
    tool: "update_panels",
    message: "Saved panel 1 of 2: p95 latency",
  }),
]

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("StepTimeline", () => {
  it("shows the live text under the working row while working", () => {
    render(
      <StepTimeline
        steps={steps}
        working
        reconnecting={false}
        liveText={"Checking the p99\nlatency of checkout"}
      />
    )
    const text = screen.getByText(/Checking the p99/)
    expect(text.tagName).toBe("P")
    expect(text).toHaveTextContent("latency of checkout")
    expect(text).toHaveAttribute("aria-live", "off")
  })

  it("hides the live text when the run is not working or has none", () => {
    const { rerender } = render(
      <StepTimeline
        steps={steps}
        working={false}
        reconnecting={false}
        liveText="stale text"
      />
    )
    expect(screen.queryByText("stale text")).toBeNull()
    rerender(
      <StepTimeline
        steps={steps}
        working
        reconnecting={false}
        liveText={null}
      />
    )
    expect(screen.queryByText("stale text")).toBeNull()
    expect(document.querySelectorAll("p")).toHaveLength(0)
  })

  it("renders markup in the live text as literal text", () => {
    const markup = '<img src=x onerror="alert(1)"> **bold** <b>tag</b>'
    const { container } = render(
      <StepTimeline
        steps={steps}
        working
        reconnecting={false}
        liveText={markup}
      />
    )
    expect(screen.getByText(markup)).toBeInTheDocument()
    expect(container.querySelector("img, b, strong")).toBeNull()
  })

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

  it("ends the steps with the request id and a copy button", async () => {
    const user = userEvent.setup()
    const writeText = vi.fn(() => Promise.resolve())
    vi.stubGlobal("navigator", { clipboard: { writeText } })
    render(
      <StepTimeline
        steps={steps}
        working={false}
        reconnecting={false}
        requestId={requestId}
      />
    )
    expect(screen.getByText("Request a77cc709")).toBeInTheDocument()
    await user.click(screen.getByRole("button", { name: "Copy request ID" }))
    expect(writeText).toHaveBeenCalledWith(requestId)
    expect(screen.queryByRole("link", { name: "Open in a new tab" })).toBeNull()
  })

  it("links the request to a new tab in the pane layout", async () => {
    renderWithProviders(
      <StepTimeline
        steps={steps}
        working={false}
        reconnecting={false}
        requestId={requestId}
        pane
      />,
      { route: { path: "/b/$requestId", url: `/b/${requestId}` } }
    )
    const link = await screen.findByRole("link", { name: "Open in a new tab" })
    expect(link).toHaveAttribute("href", `/c/${requestId}`)
    expect(link).toHaveAttribute("target", "_blank")
  })

  it("leaves the request id out when none is given", () => {
    render(<StepTimeline steps={steps} working={false} reconnecting={false} />)
    expect(screen.queryByText(/^Request /)).toBeNull()
  })
})
