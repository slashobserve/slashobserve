import { screen, waitFor } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { datasourceKeys, observeKeys } from "@/api/keys"
import type { ObserveRequest } from "@/observe/queries"
import { renderWithProviders, testQueryClient } from "@/test/render"

import { Composer } from "./composer"
import { ungated } from "./delivery"
import type { Subject } from "./send-plan"
import { useComposer, type SentResult } from "./use-composer"

const session = vi.hoisted(() => ({ role: "editor" }))
vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  useRouteContext: () => ({ principal: { id: "u1", role: session.role } }),
}))

const toasts = vi.hoisted(() => ({ message: vi.fn(), error: vi.fn() }))
vi.mock("sonner", () => ({ toast: toasts }))

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

function ChatComposer({
  subject = null,
  chatId = null,
  initialText,
  runningRequestId,
  onSent,
}: {
  subject?: Subject | null
  chatId?: string | null
  initialText?: string
  runningRequestId?: string
  onSent: (sent: SentResult) => void
}) {
  const model = useComposer({
    subject,
    place: { kind: "chat", chatId },
    gate: ungated,
    initialText,
    runningRequestId,
    onSent,
  })
  return (
    <Composer model={model} layout="page" placeholder="Describe a dashboard" />
  )
}

function renderComposer(
  props: Omit<Parameters<typeof ChatComposer>[0], "onSent"> = {},
  agentLoaded = true
) {
  const queryClient = testQueryClient()
  if (agentLoaded) {
    queryClient.setQueryData(observeKeys.agent(), {
      available: true,
      provider: "anthropic",
      model: "claude-haiku-5-5",
      effort: "medium",
    })
  }
  queryClient.setQueryData(datasourceKeys.list(), { datasources: [] })
  if (props.runningRequestId !== undefined) {
    queryClient.setQueryData(observeKeys.request(props.runningRequestId), {
      id: runningId,
      status: "running",
      can_cancel: true,
      cancel_requested_at: null,
    } as Partial<ObserveRequest> as ObserveRequest)
  }
  const onSent = vi.fn<(sent: SentResult) => void>()
  const result = renderWithProviders(
    <ChatComposer {...props} onSent={onSent} />,
    { queryClient, route: { path: "/", url: "/" } }
  )
  return { ...result, onSent }
}

async function box() {
  return screen.findByRole("textbox", { name: "Message the agent" })
}

async function sentBody(fetch: ReturnType<typeof stubCreate>, call = 0) {
  return (await fetch.mock.calls[call][0].clone().json()) as Record<
    string,
    unknown
  >
}

beforeEach(() => {
  localStorage.clear()
  session.role = "editor"
  toasts.message.mockReset()
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("Composer in a chat", () => {
  it("shows the keyboard hint to everyone, not only to screen readers", async () => {
    renderComposer()
    const hint = await screen.findByText(/Enter sends\. Shift\+Enter/)
    expect(hint).not.toHaveClass("sr-only")
  })

  it("names who reads the message, with chips that do not turn", async () => {
    renderComposer()
    await box()
    expect(screen.getByText("To: Observe Agent")).toBeVisible()
    expect(screen.getByText("Visible to: only you")).toBeVisible()
    expect(screen.queryByRole("button", { name: /To:/ })).toBeNull()
  })

  it("adds a new line on Shift+Enter and sends nothing", async () => {
    const fetch = stubCreate()
    const { user, onSent } = renderComposer()
    await user.type(await box(), "first{Shift>}{Enter}{/Shift}second")
    expect(await box()).toHaveValue("first\nsecond")
    expect(fetch).not.toHaveBeenCalled()
    expect(onSent).not.toHaveBeenCalled()
  })

  it("sends the trimmed prompt on Enter as an ask and opens the chat from Home", async () => {
    const fetch = stubCreate()
    const { user, onSent } = renderComposer()
    await user.type(await box(), "  checkout health  {Enter}")
    await waitFor(() => expect(onSent).toHaveBeenCalledOnce())
    expect(onSent.mock.calls[0][0]).toMatchObject({
      via: "chat",
      opens: "chat",
    })
    expect(await sentBody(fetch)).toMatchObject({
      prompt: "checkout health",
      kind: "ask",
      client_id: expect.any(String),
    })
    expect(await box()).toHaveValue("")
  })

  it("continues an open chat in place with a new client id per message", async () => {
    const fetch = stubCreate()
    const { user, onSent } = renderComposer({ chatId: "c1" })
    await user.type(await box(), "one{Enter}")
    await waitFor(() => expect(onSent).toHaveBeenCalledOnce())
    await user.type(await box(), "two{Enter}")
    await waitFor(() => expect(onSent).toHaveBeenCalledTimes(2))
    const first = await sentBody(fetch, 0)
    const second = await sentBody(fetch, 1)
    expect(first).toMatchObject({ chat_id: "c1", kind: "ask" })
    expect(onSent.mock.calls[0][0]).toMatchObject({ opens: "same" })
    expect(first.client_id).not.toBe(second.client_id)
  })

  it("asks about the dashboard the chat is about without changing it", async () => {
    const fetch = stubCreate()
    const { user, onSent } = renderComposer({
      subject: { kind: "dashboard", id: dashboardId },
      chatId: "c1",
    })
    await user.type(await box(), "add a p99 panel{Enter}")
    await waitFor(() => expect(onSent).toHaveBeenCalledOnce())
    expect(await sentBody(fetch)).toMatchObject({
      kind: "ask",
      dashboard_id: dashboardId,
      chat_id: "c1",
    })
  })

  it("recalls the sent prompt with the up arrow", async () => {
    stubCreate()
    const { user, onSent } = renderComposer()
    await user.type(await box(), "checkout health{Enter}")
    await waitFor(() => expect(onSent).toHaveBeenCalledOnce())
    await user.keyboard("{ArrowUp}")
    expect(await box()).toHaveValue("checkout health")
  })

  it.each([
    ["editor", true],
    ["admin", true],
    ["commenter", false],
    ["viewer", false],
  ])("offers /investigate to a %s: %s", async (role, offered) => {
    session.role = role
    const { user } = renderComposer()
    await user.type(await box(), "/inv")
    const hint = screen.queryByText(
      "asks a question and answers with a finding"
    )
    if (offered) expect(hint).toBeInTheDocument()
    else expect(hint).not.toBeInTheDocument()
  })

  it("turns a typed /investigate into a chip and sends an investigation", async () => {
    const fetch = stubCreate()
    const { user, onSent } = renderComposer()
    await user.type(await box(), "/investigate ")
    expect(await box()).toHaveValue("")
    expect(screen.getByRole("list", { name: "Context" })).toHaveTextContent(
      "Investigate"
    )
    await user.type(await box(), "why is checkout slow{Enter}")
    await waitFor(() => expect(onSent).toHaveBeenCalledOnce())
    expect(await sentBody(fetch)).toMatchObject({
      prompt: "why is checkout slow",
      kind: "investigate",
    })
  })

  it("offers no /revise", async () => {
    const { user } = renderComposer({
      subject: { kind: "dashboard", id: dashboardId },
    })
    await user.type(await box(), "/revise ")
    expect(await box()).toHaveValue("/revise ")
  })

  it("shows the model and effort in the footer", async () => {
    renderComposer()
    expect(
      await screen.findByText("claude-haiku-5-5, medium effort")
    ).toBeInTheDocument()
  })

  it("asks for a question or a dashboard when empty, and for a change on a dashboard", async () => {
    stubCreate()
    const { user, unmount } = renderComposer()
    await user.click(await screen.findByRole("button", { name: "Send" }))
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Ask a question or describe a dashboard first."
    )
    unmount()
    const next = renderComposer({
      subject: { kind: "dashboard", id: dashboardId },
    })
    await next.user.click(await screen.findByRole("button", { name: "Send" }))
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Describe the change you want first."
    )
  })

  it("counts characters near the limit and marks an over-long prompt", async () => {
    const { user } = renderComposer()
    const field = await box()
    await user.click(field)
    await user.paste("a".repeat(3600))
    expect(screen.getByText("3600 of 4000")).toBeInTheDocument()
    await user.paste("a".repeat(401))
    expect(screen.getByText("4001 of 4000, too long")).toBeInTheDocument()
    expect(field).toHaveAttribute("aria-invalid", "true")
  })

  it("opens with a prefilled prompt and sends nothing until the person does", async () => {
    const fetch = stubCreate()
    renderComposer({ initialText: "Why is GET /api/products slow?" })
    expect(await box()).toHaveValue("Why is GET /api/products slow?")
    expect(fetch).not.toHaveBeenCalled()
  })

  it("offers Stop for the running request", async () => {
    stubCreate()
    renderComposer({ runningRequestId: runningId })
    expect(
      await screen.findByRole("button", { name: "Stop the run" })
    ).toBeInTheDocument()
  })

  it("says so when Enter is pressed before the agent is known", async () => {
    const fetch = vi.fn(() => new Promise<Response>(() => undefined))
    vi.stubGlobal("fetch", fetch)
    const { user, onSent } = renderComposer(
      { initialText: "Why is it slow?" },
      false
    )
    await user.type(await box(), "{Enter}")
    expect(toasts.message).toHaveBeenCalledWith(
      "Still connecting to the agent. Send again in a moment."
    )
    expect(onSent).not.toHaveBeenCalled()
  })
})
