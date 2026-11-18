import { screen } from "@testing-library/react"
import { describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { TurnFailure } from "./turn-failure"

const base = {
  id: "a77cc709-1b2c-4d3e-8f4a-5b6c7d8e9f01",
  source: "observe" as const,
  dashboard_id: null,
  error: "the run was canceled",
  continued_by: null,
  retried_by: null,
  allowed_actions: ["retry" as const],
}

function renderFailure(failureCode: string) {
  return renderWithProviders(
    <TurnFailure
      request={{ ...base, failure_code: failureCode }}
      onRetried={vi.fn()}
    />,
    { route: { path: "/", url: "/" } }
  )
}

describe("TurnFailure", () => {
  it("shows a stopped run as a calm status, once", async () => {
    renderFailure("canceled")
    expect(await screen.findByRole("status")).toHaveTextContent(
      "You stopped this run"
    )
    expect(screen.queryByRole("alert")).toBeNull()
    expect(screen.queryByText(/could not be built/i)).toBeNull()
    expect(screen.queryAllByText(/was canceled/i)).toHaveLength(0)
    expect(
      screen.getByRole("button", { name: "Try again" })
    ).toBeInTheDocument()
  })

  it("offers saving or continuing only while a build is left", async () => {
    renderWithProviders(
      <TurnFailure
        request={{
          ...base,
          failure_code: "canceled",
          allowed_actions: ["retry", "continue", "keep_build"],
        }}
        onRetried={vi.fn()}
      />,
      { route: { path: "/", url: "/" } }
    )
    expect(
      await screen.findByText(/Save what was built, continue it/)
    ).toBeInTheDocument()
  })

  it("says nothing was built when the run stopped before a panel", async () => {
    renderFailure("canceled")
    expect(await screen.findByText("Try again.")).toBeInTheDocument()
    expect(screen.queryByText(/Save what was built/)).toBeNull()
  })

  it("points to the retry and offers no second one", async () => {
    renderWithProviders(
      <TurnFailure
        request={{
          ...base,
          failure_code: "canceled",
          retried_by: "b11cc709-1b2c-4d3e-8f4a-5b6c7d8e9f02",
          allowed_actions: [],
        }}
        onRetried={vi.fn()}
      />,
      { route: { path: "/", url: "/" } }
    )
    expect(
      await screen.findByRole("link", { name: "Tried again in a new run" })
    ).toHaveAttribute("href", "/c/b11cc709-1b2c-4d3e-8f4a-5b6c7d8e9f02")
    expect(screen.queryByRole("button", { name: "Try again" })).toBeNull()
  })

  it("keeps a real failure as an alert with its reason", async () => {
    renderFailure("no_progress")
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "The dashboard could not be built"
    )
  })
})
