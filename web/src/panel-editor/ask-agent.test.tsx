import { screen, waitFor } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { observeKeys } from "@/api/keys"
import { renderWithProviders, testQueryClient } from "@/test/render"

import { AskAgent } from "./ask-agent"
import type { EditablePanel } from "./draft"

const dashboardId = "2f0f2c1e-6a53-4a35-8a0e-1d3b0d1f7a11"
const draft = {
  id: "p1",
  type: "stat",
  title: "Errors",
  targets: [{ refId: "A", expr: "errors" }],
} as unknown as EditablePanel

type Route = { status: number; body: unknown }

function json({ status, body }: Route) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  })
}

function stubServer(poll: () => Route) {
  const created: unknown[] = []
  let release: () => void = () => {}
  const held = new Promise<void>((resolve) => {
    release = resolve
  })
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      if (request.method === "POST") {
        created.push(await request.clone().json())
        await held
        return json({
          status: 202,
          body: { suggestion: { id: "s1", status: "queued" } },
        })
      }
      return json(poll())
    })
  )
  return { created, release }
}

async function renderAsk(dispatch = vi.fn()) {
  const queryClient = testQueryClient()
  queryClient.setQueryData(observeKeys.agent(), { available: true })
  const view = renderWithProviders(
    <AskAgent
      dashboardId={dashboardId}
      version={3}
      draft={draft}
      dispatch={dispatch}
      blockedReason={null}
    />,
    { queryClient, route: { path: "/", url: "/" } }
  )
  await screen.findByRole("button", { name: "Suggest" })
  return { ...view, dispatch }
}

const field = () =>
  screen.getByRole("textbox", { name: "Ask the agent to change this query" })
const suggest = () => screen.getByRole("button", { name: "Suggest" })

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("AskAgent", () => {
  it("starts one run for a double click", async () => {
    const { created, release } = stubServer(() => ({
      status: 200,
      body: { suggestion: { id: "s1", status: "running", steps: [] } },
    }))
    const { user } = await renderAsk()
    await user.type(field(), "only 5xx")
    await user.dblClick(suggest())
    release()
    await waitFor(() => expect(created).toHaveLength(1))
    await screen.findByRole("region", { name: "Agent suggestion" })
    expect(created).toHaveLength(1)
    expect(suggest()).toBeDisabled()
  })

  it("frees Suggest when the suggestion can no longer be read", async () => {
    const { release } = stubServer(() => ({
      status: 404,
      body: { error: "suggestion not found", code: "not_found" },
    }))
    const { user } = await renderAsk()
    await user.type(field(), "only 5xx")
    await user.click(suggest())
    release()
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Suggestion not found."
    )
    expect(suggest()).toBeEnabled()
    expect(field()).not.toHaveAttribute("aria-invalid", "true")
    expect(
      screen.queryByRole("region", { name: "Agent suggestion" })
    ).toBeNull()
  })

  it("marks the field invalid for an empty prompt until it is typed in", async () => {
    stubServer(() => ({ status: 500, body: {} }))
    const { user } = await renderAsk()
    await user.click(suggest())
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Describe the change you want first."
    )
    expect(field()).toHaveAttribute("aria-invalid", "true")
    await user.type(field(), "x")
    expect(screen.queryByRole("alert")).toBeNull()
    expect(field()).toHaveAttribute("aria-invalid", "false")
  })

  it("applies an accepted change and says so to screen readers", async () => {
    const proposed = { ...draft, title: "5xx errors" }
    const { release } = stubServer(() => ({
      status: 200,
      body: {
        suggestion: {
          id: "s1",
          status: "succeeded",
          steps: [],
          panel: proposed,
        },
      },
    }))
    const { user, dispatch } = await renderAsk()
    await user.type(field(), "only 5xx")
    await user.click(suggest())
    release()
    await user.click(await screen.findByRole("button", { name: "Accept" }))
    expect(dispatch).toHaveBeenCalledWith(
      expect.objectContaining({ type: "replace" })
    )
    expect(screen.getByRole("status")).toHaveTextContent(
      "Applied the agent's change. Save to keep it."
    )
    expect(suggest()).toBeEnabled()
  })
})
