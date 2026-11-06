import { render, screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { Button } from "@/components/ui/button"

import { EmptyState } from "./empty-state"

describe("EmptyState", () => {
  it("renders the title, the body and the action", () => {
    render(
      <EmptyState
        title="No dashboards yet"
        action={<Button>New dashboard</Button>}
      >
        Ask the agent for one or import from Grafana.
      </EmptyState>
    )
    expect(screen.getByText("No dashboards yet").tagName).toBe("P")
    expect(
      screen.getByText("Ask the agent for one or import from Grafana.")
    ).toBeInTheDocument()
    expect(
      screen.getByRole("button", { name: "New dashboard" })
    ).toBeInTheDocument()
    expect(screen.queryByRole("heading")).toBeNull()
  })

  it("shows a status-ok check for the ok tone", () => {
    const { container } = render(
      <EmptyState tone="ok" title="All datasources are healthy" />
    )
    const icon = container.querySelector("svg")
    expect(icon).toHaveClass("text-status-ok")
    expect(icon).toHaveAttribute("aria-hidden", "true")
  })

  it("prefers a given icon over the ok check", () => {
    const { container } = render(
      <EmptyState
        tone="ok"
        title="Nothing here"
        icon={<svg data-testid="given" />}
      />
    )
    expect(screen.getByTestId("given")).toBeInTheDocument()
    expect(container.querySelectorAll("svg")).toHaveLength(1)
  })

  it("renders no icon by default", () => {
    const { container } = render(<EmptyState title="No comments yet" />)
    expect(container.querySelector("svg")).toBeNull()
  })
})
