import { screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { renderWithProviders } from "@/test/render"

import { RunStarted } from "./run-started"

const runId = "6f1e2d3c-4b5a-4968-8776-655443322110"

function renderLine(props: Parameters<typeof RunStarted>[0]) {
  return renderWithProviders(<RunStarted {...props} />, {
    route: { path: "/", url: "/" },
  })
}

describe("RunStarted", () => {
  it("says the Observe Agent is working and links to the run", async () => {
    renderLine({
      run: { id: runId, status: "queued", assigned_kind: "builtin" },
      refusal: null,
      agentName: "Observe Agent",
    })
    expect(await screen.findByRole("status")).toHaveTextContent(
      "Observe Agent is working on it."
    )
    expect(screen.getByRole("link", { name: "View the run" })).toHaveAttribute(
      "href",
      `/c/${runId}`
    )
  })

  it.each(["grant", "service_account"] as const)(
    "names a %s agent and offers no link",
    async (assigned_kind) => {
      renderLine({
        run: { id: runId, status: "queued", assigned_kind },
        refusal: null,
        agentName: "Claude Code",
      })
      expect(await screen.findByRole("status")).toHaveTextContent(
        "Claude Code was asked."
      )
      expect(screen.queryByRole("link")).toBeNull()
    }
  )

  it("shows the server's refusal as a sentence", async () => {
    renderLine({
      run: null,
      refusal: {
        code: "too_many_runs",
        message: "this dashboard already has 5 runs waiting",
      },
      agentName: "Observe Agent",
    })
    expect(await screen.findByRole("status")).toHaveTextContent(
      "This dashboard already has 5 runs waiting."
    )
    expect(screen.queryByRole("link")).toBeNull()
  })

  it("renders nothing when no run started and none was refused", async () => {
    const { container } = renderLine({
      run: null,
      refusal: null,
      agentName: "",
    })
    expect(container.textContent).toBe("")
  })
})
