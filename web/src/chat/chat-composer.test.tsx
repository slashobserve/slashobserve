import { screen, waitFor } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { datasourceKeys, observeKeys } from "@/api/keys"
import type { ObserveRequest } from "@/observe/queries"
import { renderWithProviders, testQueryClient } from "@/test/render"

import { ChatComposer } from "./chat-composer"

const dashboardId = "2f0f2c1e-6a53-4a35-8a0e-1d3b0d1f7a11"
const runningId = "7a1d2b3c-4d5e-4f60-8a9b-0c1d2e3f4a5b"

function stubCreate() {
  const fetch = vi.fn<(request: Request) => Promise<Response>>(
    async (request) => {
      const body = (await request.clone().json()) as { prompt: string }
      return new Response(
        JSON.stringify({ request: { id: "r1", prompt: body.prompt } }),
        { status: 202, headers: { "Content-Type": "application/json" } }
      )
    }
  )
  vi.stubGlobal("fetch", fetch)
  return fetch
}

function stubRunning(request: Partial<ObserveRequest>) {
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(JSON.stringify({ request }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        })
    )
  )
}

const running: Partial<ObserveRequest> = {
  id: runningId,
  status: "running",
  can_cancel: true,
  cancel_requested_at: null,
}

function renderComposer(
  props: { dashboardId?: string; runningRequestId?: string } = {}
) {
  const queryClient = testQueryClient()
  queryClient.setQueryData(observeKeys.agent(), {
    available: true,
    provider: "anthropic",
    model: "claude-haiku-5-5",
    effort: "medium",
  })
  queryClient.setQueryData(datasourceKeys.list(), { datasources: [] })
  if (props.runningRequestId !== undefined) {
    queryClient.setQueryData(
      observeKeys.request(props.runningRequestId),
      running as ObserveRequest
    )
  }
  const onSent = vi.fn()
  const result = renderWithProviders(
    <ChatComposer
      rows={2}
      placeholder="Describe a dashboard"
      onSent={onSent}
      {...props}
    />,
    { queryClient, route: { path: "/", url: "/" } }
  )
  return { ...result, onSent }
}

async function box() {
  return screen.findByRole("textbox", { name: "Message the agent" })
}

beforeEach(() => {
  localStorage.clear()
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("ChatComposer", () => {
  it("adds a new line on Shift+Enter and sends nothing", async () => {
    const fetch = stubCreate()
    const { user, onSent } = renderComposer()
    await user.type(await box(), "first{Shift>}{Enter}{/Shift}second")
    expect(await box()).toHaveValue("first\nsecond")
    expect(fetch).not.toHaveBeenCalled()
    expect(onSent).not.toHaveBeenCalled()
  })

  it("sends the trimmed prompt on Enter and opens a build without a dashboard", async () => {
    const fetch = stubCreate()
    const { user, onSent } = renderComposer()
    await user.type(await box(), "  checkout health  {Enter}")
    await waitFor(() => expect(onSent).toHaveBeenCalledOnce())
    expect(onSent.mock.calls[0][1]).toBe("build")
    const sent = (await fetch.mock.calls[0][0].json()) as { prompt: string }
    expect(sent.prompt).toBe("checkout health")
    expect(await box()).toHaveValue("")
  })

  it("recalls the sent prompt with the up arrow", async () => {
    stubCreate()
    const { user, onSent } = renderComposer()
    await user.type(await box(), "checkout health{Enter}")
    await waitFor(() => expect(onSent).toHaveBeenCalledOnce())
    await user.keyboard("{ArrowUp}")
    expect(await box()).toHaveValue("checkout health")
  })

  it("turns a typed /revise into a chip and empties the box", async () => {
    stubCreate()
    const { user } = renderComposer({ dashboardId })
    await user.type(await box(), "/revise ")
    expect(await box()).toHaveValue("")
    expect(screen.getByRole("list", { name: "Context" })).toHaveTextContent(
      "Revise"
    )
    expect(
      screen.getByRole("button", { name: "Remove Revise" })
    ).toBeInTheDocument()
  })

  it("shows the model and effort in the footer", async () => {
    renderComposer()
    expect(
      await screen.findByText("claude-haiku-5-5, medium effort")
    ).toBeInTheDocument()
  })

  it("offers Stop for the running request", async () => {
    stubRunning(running)
    renderComposer({ runningRequestId: runningId })
    expect(
      await screen.findByRole("button", { name: "Stop the run" })
    ).toBeInTheDocument()
  })
})
