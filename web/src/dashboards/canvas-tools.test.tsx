import { screen } from "@testing-library/react"
import { createRef } from "react"
import { describe, expect, it } from "vitest"

import { observeKeys } from "@/api/keys"
import { renderWithProviders, testQueryClient } from "@/test/render"

import { CanvasTools } from "./canvas-tools"

async function renderTools(
  permissions: { comment: boolean; edit: boolean },
  agentAvailable: boolean
) {
  const queryClient = testQueryClient()
  queryClient.setQueryData(observeKeys.agent(), {
    available: agentAvailable,
    provider: agentAvailable ? "anthropic" : null,
    model: agentAvailable ? "claude-haiku-5-5" : null,
    effort: null,
  })
  renderWithProviders(
    <>
      <p>page</p>
      <CanvasTools permissions={permissions} idle composerRef={createRef()} />
    </>,
    { queryClient, route: { path: "/", url: "/" } }
  )
  await screen.findByText("page")
}

describe("CanvasTools", () => {
  it("shows Select and Comment to a Commenter", async () => {
    await renderTools({ comment: true, edit: false }, true)
    expect(
      screen.getByRole("toolbar", { name: "Canvas tools" })
    ).toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Select" })).toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Comment" })).toBeInTheDocument()
    expect(
      screen.queryByRole("button", { name: "Ask" })
    ).not.toBeInTheDocument()
  })

  it("shows Ask to an Editor with an agent", async () => {
    await renderTools({ comment: false, edit: true }, true)
    expect(screen.getByRole("button", { name: "Ask" })).toBeInTheDocument()
  })

  it.each([
    ["a Viewer", { comment: false, edit: false }, true],
    [
      "an Editor with no agent and no comment access",
      { comment: false, edit: true },
      false,
    ],
  ])("hides the whole bar for %s", async (_who, permissions, agent) => {
    await renderTools(permissions, agent)
    expect(screen.queryByRole("toolbar")).not.toBeInTheDocument()
    expect(
      screen.queryByRole("button", { name: "Select" })
    ).not.toBeInTheDocument()
  })
})
