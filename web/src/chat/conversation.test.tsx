import { screen, waitFor } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { datasourceKeys, observeKeys } from "@/api/keys"
import type { ObserveRequest } from "@/observe/queries"
import { renderWithProviders, testQueryClient } from "@/test/render"

import { Conversation } from "./conversation"

vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  useRouteContext: () => ({ principal: { id: "u1", role: "editor" } }),
}))

const rootId = "5b0c1d2e-3f40-4a51-8b62-7c8d9e0f1a2b"
const dashboardId = "2f0f2c1e-6a53-4a35-8a0e-1d3b0d1f7a11"

const root = {
  id: rootId,
  chat_id: rootId,
  prompt: "why is p99 up?",
  status: "succeeded",
  state: "succeeded",
  intent: "ask",
  kind: "investigate",
  dashboard_id: dashboardId,
  observation_id: null,
  read_only: false,
  allowed_actions: [],
  requested_by: { kind: "user", id: "u1", name: "Ada" },
  created_at: "2026-11-21T10:00:00Z",
  finding: null,
  steps: [],
} as Partial<ObserveRequest> as ObserveRequest

function stubServer() {
  const fetch = vi.fn<(request: Request) => Promise<Response>>(
    async (request) => {
      const url = new URL(request.url)
      if (request.method === "POST" && url.pathname === "/api/v1/observe") {
        const body = (await request.clone().json()) as { prompt: string }
        return Response.json(
          { request: { ...root, id: "r2", prompt: body.prompt } },
          { status: 202 }
        )
      }
      return Response.json(
        { code: "not_found", message: "not found" },
        { status: 404 }
      )
    }
  )
  vi.stubGlobal("fetch", fetch)
  return fetch
}

function renderConversation() {
  const queryClient = testQueryClient()
  queryClient.setQueryData(observeKeys.request(rootId), root)
  queryClient.setQueryData(observeKeys.agent(), {
    available: true,
    provider: "anthropic",
    model: "claude-haiku-5-5",
    effort: "medium",
  })
  queryClient.setQueryData(datasourceKeys.list(), { datasources: [] })
  return renderWithProviders(
    <Conversation requestId={rootId} layout="page" />,
    { queryClient, route: { path: "/", url: "/" } }
  )
}

afterEach(() => vi.unstubAllGlobals())

describe("Conversation", () => {
  it("sends a follow-up in a dashboard chat as a read-only ask", async () => {
    const fetch = stubServer()
    const { user } = renderConversation()
    const box = await screen.findByRole("textbox", {
      name: "Message the agent",
    })
    await user.type(box, "and yesterday?{Enter}")
    const posted = () =>
      fetch.mock.calls
        .map(([request]) => request)
        .find(
          (request) =>
            request.method === "POST" &&
            new URL(request.url).pathname === "/api/v1/observe"
        )
    await waitFor(() => expect(posted()).toBeDefined())
    const body = (await posted()!.clone().json()) as Record<string, unknown>
    expect(body).toMatchObject({
      prompt: "and yesterday?",
      kind: "ask",
      dashboard_id: dashboardId,
      chat_id: rootId,
    })
  })
})
