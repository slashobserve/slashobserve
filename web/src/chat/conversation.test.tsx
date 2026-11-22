import { useParams } from "@tanstack/react-router"
import { screen, waitFor } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { datasourceKeys, observeKeys } from "@/api/keys"
import { noModelReviseText } from "@/conversation/delivery"
import type { Finding } from "@/findings/queries"
import type { ObserveRequest } from "@/observe/queries"
import { redactedRun } from "@/test/agent-request"
import { renderWithProviders, testQueryClient } from "@/test/render"

import { Conversation } from "./conversation"
import type { ChatPanel } from "./panel-target"

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
  investigation: false,
  dashboard_id: dashboardId,
  observation_id: null,
  subject_observation_id: null,
  read_only: false,
  allowed_actions: [],
  requested_by: { kind: "user", id: "u1", name: "Ada" },
  created_at: "2026-11-21T10:00:00Z",
  finding: null,
  steps: [],
} as Partial<ObserveRequest> as ObserveRequest

const observationId = "8d9e0f1a-2b3c-4d5e-8f60-718293a4b5c6"

function chatAbout(subject: {
  dashboard_id?: string
  observation_id?: string
}) {
  return {
    chat: {
      id: rootId,
      title: "why is p99 up?",
      dashboard_id: null,
      observation_id: null,
      ...subject,
      pinned_at: null,
      created_at: "2026-11-21T10:00:00Z",
      last_turn_at: "2026-11-21T10:00:00Z",
      turns: 1,
      last_turn: null,
      owner: { kind: "user", name: "Ada" },
      read_only: false,
    },
    turns: [] as ObserveRequest[],
  }
}

type Answer = { status: number; body: unknown }

function stubServer(
  chat: ReturnType<typeof chatAbout> | null = chatAbout({
    dashboard_id: dashboardId,
  }),
  observe?: Answer,
  requests: readonly ObserveRequest[] = []
) {
  const fetch = vi.fn<(request: Request) => Promise<Response>>(
    async (request) => {
      const url = new URL(request.url)
      if (chat !== null && url.pathname === `/api/v1/chats/${rootId}`) {
        return Response.json(chat)
      }
      const known = requests.find(
        ({ id }) => url.pathname === `/api/v1/observe/${id}`
      )
      if (request.method === "GET" && known !== undefined) {
        return Response.json(known)
      }
      if (request.method === "POST" && url.pathname === "/api/v1/observe") {
        if (observe !== undefined) {
          return Response.json(observe.body, { status: observe.status })
        }
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

function ChatPage() {
  const { requestId } = useParams({ strict: false }) as { requestId: string }
  return <Conversation key={requestId} requestId={requestId} layout="page" />
}

function seededClient(request: ObserveRequest) {
  const queryClient = testQueryClient()
  queryClient.setQueryData(observeKeys.request(rootId), request)
  queryClient.setQueryData(observeKeys.agent(), {
    available: true,
    provider: "anthropic",
    model: "claude-haiku-5-5",
    effort: "medium",
  })
  queryClient.setQueryData(datasourceKeys.list(), { datasources: [] })
  return queryClient
}

function renderChatPage(request: ObserveRequest) {
  return renderWithProviders(<ChatPage />, {
    queryClient: seededClient(request),
    route: { path: "/c/$requestId", url: `/c/${rootId}` },
  })
}

function renderConversation(
  request: ObserveRequest = root,
  panel: ChatPanel | null = null,
  layout: "page" | "pane" = "page"
) {
  const queryClient = seededClient(request)
  queryClient.setQueryData(datasourceKeys.list(), {
    datasources: [{ uid: "prom", type: "prometheus", healthy: true }],
  })
  return renderWithProviders(
    <Conversation requestId={rootId} layout={layout} panel={panel} />,
    { queryClient, route: { path: "/", url: "/" } }
  )
}

afterEach(() => vi.unstubAllGlobals())

const finding: Finding = {
  id: "f1",
  observation_id: null,
  agent_request_id: rootId,
  dashboard_id: null,
  folder_id: null,
  ask_on: null,
  restricted: false,
  title: "p99 rose after the deploy",
  markdown: "p99 rose after the deploy.",
  evidence: [],
  from: "2026-11-21T09:00:00Z",
  to: "2026-11-21T10:00:00Z",
  annotation_id: null,
  created_by: { kind: "user", id: "u1", name: "Ada" },
  actor: {
    kind: "user",
    id: "u1",
    name: "Ada",
    origin: "web",
    client: null,
    run_id: rootId,
    label: "Observe Agent for Ada",
  },
  created_at: "2026-11-21T10:01:00Z",
  url: null,
  suggested_change: "Add a p99 panel",
  observe_worthy: false,
}

function answered(patch: Partial<ObserveRequest> = {}) {
  return {
    ...root,
    result_kind: "answer",
    finding,
    shared_thread_id: null,
    ...patch,
  } as ObserveRequest
}

async function openAnswerMenu(
  user: ReturnType<typeof renderConversation>["user"]
) {
  await user.click(
    await screen.findByRole("button", { name: "Answer actions" })
  )
}

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

  it("asks about the panel it was opened on, with its chip and the focus in the box", async () => {
    const fetch = stubServer()
    const onRemove = vi.fn()
    const { user } = renderConversation(root, {
      id: "p1",
      title: "Latency",
      onRemove,
    })
    const box = await screen.findByRole("textbox", {
      name: "Message the agent",
    })
    await waitFor(() => expect(box).toHaveFocus())
    expect(screen.getByRole("list", { name: "Context" })).toHaveTextContent(
      "Latency"
    )
    await user.type(box, "why the spike?{Enter}")
    const posted = await vi.waitFor(() => {
      const found = fetch.mock.calls.find(
        ([request]) =>
          request.method === "POST" &&
          new URL(request.url).pathname === "/api/v1/observe"
      )
      expect(found).toBeDefined()
      return found![0]
    })
    expect(await posted.clone().json()).toMatchObject({
      dashboard_id: dashboardId,
      panel_id: "p1",
    })
    await user.click(screen.getByRole("button", { name: "Remove Latency" }))
    expect(onRemove).toHaveBeenCalledTimes(1)
  })

  async function sentFollowUp(fetch: ReturnType<typeof stubServer>) {
    const { user } = renderConversation({
      ...root,
      dashboard_id: null,
      kind: "build",
    } as ObserveRequest)
    const box = await screen.findByRole("textbox", {
      name: "Message the agent",
    })
    await vi.waitFor(() =>
      expect(
        fetch.mock.calls.some(
          ([request]) =>
            new URL(request.url).pathname === `/api/v1/chats/${rootId}`
        )
      ).toBe(true)
    )
    await user.type(box, "and yesterday?{Enter}")
    const posted = await vi.waitFor(() => {
      const found = fetch.mock.calls.find(
        ([request]) =>
          request.method === "POST" &&
          new URL(request.url).pathname === "/api/v1/observe"
      )
      expect(found).toBeDefined()
      return found![0]
    })
    return (await posted.clone().json()) as Record<string, unknown>
  }

  it("asks about the dashboard the chat became about after its first turn", async () => {
    const fetch = stubServer(chatAbout({ dashboard_id: dashboardId }))
    expect(await sentFollowUp(fetch)).toMatchObject({
      kind: "ask",
      chat_id: rootId,
      dashboard_id: dashboardId,
    })
  })

  it("asks about the observation the chat is about", async () => {
    const fetch = stubServer(chatAbout({ observation_id: observationId }))
    const body = await sentFollowUp(fetch)
    expect(body).toMatchObject({
      kind: "ask",
      chat_id: rootId,
      observation_id: observationId,
    })
    expect(body).not.toHaveProperty("dashboard_id")
  })

  it("offers the asker the share choices on an answer about an observation", async () => {
    stubServer(chatAbout({ observation_id: observationId }))
    const { user } = renderConversation(answered({ dashboard_id: null }))
    await openAnswerMenu(user)
    expect(
      await screen.findByRole("menuitem", { name: /^Show to everyone/ })
    ).toHaveTextContent("writes the answer onto the page")
    expect(
      screen.getByRole("menuitem", { name: /^Share to thread/ })
    ).toBeVisible()
  })

  it("names the observation thread a shared answer is in and what Withdraw leaves", async () => {
    stubServer(chatAbout({ observation_id: observationId }))
    const { user } = renderConversation(
      answered({ dashboard_id: null, shared_thread_id: "t5" })
    )
    await openAnswerMenu(user)
    expect(
      await screen.findByRole("menuitem", { name: /^Shared in the thread/ })
    ).toHaveAttribute("href", `/o/${observationId}`)
    expect(
      screen.getByRole("menuitem", { name: /^Withdraw/ })
    ).toHaveTextContent("Blocks it already wrote on the page stay.")
  })

  it("keeps the answer card for an answer saved as an observation", async () => {
    stubServer(chatAbout({ dashboard_id: dashboardId }))
    renderConversation(
      answered({
        result_kind: "observation",
        finding: { ...finding, observation_id: "o9" },
      })
    )
    expect(
      await screen.findByRole("link", { name: "Open the observation" })
    ).toHaveAttribute("href", "/o/o9")
    expect(screen.queryByRole("link", { name: "Open observation" })).toBeNull()
  })

  it("offers sharing into the observation an answer was about after it was saved into another", async () => {
    stubServer(null)
    const { user } = renderConversation(
      answered({
        chat_id: null,
        dashboard_id: null,
        subject_observation_id: observationId,
        observation_id: "o9",
        result_kind: "observation",
        finding: { ...finding, observation_id: "o9" },
      })
    )
    expect(
      await screen.findByRole("link", { name: "Open the observation" })
    ).toHaveAttribute("href", "/o/o9")
    await openAnswerMenu(user)
    expect(
      await screen.findByRole("menuitem", { name: /^Show to everyone/ })
    ).toHaveTextContent("writes the answer onto the page")
    expect(
      screen.getByRole("menuitem", { name: /^Share to thread/ })
    ).toBeVisible()
  })

  it.each([
    [
      "saved into the observation it was about",
      {
        dashboard_id: null,
        subject_observation_id: observationId,
        observation_id: observationId,
        finding: { ...finding, observation_id: observationId },
      },
      chatAbout({ observation_id: observationId }),
    ],
    [
      "about an observation the asker can no longer read",
      {
        dashboard_id: null,
        subject_observation_id: null,
        observation_id: "o9",
        finding: { ...finding, observation_id: "o9" },
      },
      chatAbout({ observation_id: observationId }),
    ],
    [
      "about no observation",
      {
        subject_observation_id: null,
        observation_id: "o9",
        finding: { ...finding, observation_id: "o9" },
      },
      chatAbout({ dashboard_id: dashboardId }),
    ],
  ])(
    "offers no sharing once an answer %s is saved",
    async (_name, patch, chat) => {
      stubServer(chat)
      const { user } = renderConversation(
        answered({ result_kind: "observation", ...patch })
      )
      await openAnswerMenu(user)
      expect(
        await screen.findByRole("menuitem", { name: /^Copy answer/ })
      ).toBeVisible()
      expect(
        screen.queryByRole("menuitem", { name: /^Show to everyone/ })
      ).toBeNull()
      expect(
        screen.queryByRole("menuitem", { name: /^Share to thread/ })
      ).toBeNull()
    }
  )

  it("keeps the shared thread on an answer shared and then saved", async () => {
    stubServer(chatAbout({ dashboard_id: dashboardId }))
    const { user } = renderConversation(
      answered({
        result_kind: "observation",
        finding: { ...finding, observation_id: "o9" },
        shared_thread_id: "t5",
      })
    )
    await openAnswerMenu(user)
    expect(
      await screen.findByRole("menuitem", { name: /^Shared in the thread/ })
    ).toBeVisible()
    expect(screen.getByRole("menuitem", { name: /^Withdraw/ })).toBeVisible()
  })

  it("asks for the answer's suggested change by its finding, with no prompt", async () => {
    const fetch = stubServer()
    const { user } = renderConversation(answered())
    await user.click(
      await screen.findByRole("button", { name: "Revise the dashboard" })
    )
    const posted = await vi.waitFor(() => {
      const found = fetch.mock.calls.find(
        ([request]) =>
          request.method === "POST" &&
          new URL(request.url).pathname === "/api/v1/observe"
      )
      expect(found).toBeDefined()
      return found![0]
    })
    const body = (await posted.clone().json()) as Record<string, unknown>
    expect(body).toMatchObject({
      kind: "revise",
      applies_finding_id: "f1",
      dashboard_id: dashboardId,
      chat_id: rootId,
    })
    expect(body).not.toHaveProperty("prompt")
  })

  const revision = {
    ...root,
    id: "r2",
    kind: "build",
    intent: "apply",
    status: "queued",
    state: "queued",
    prompt: "Add a p99 panel",
    build: {
      saves: 0,
      panels: 0,
      planned_panels: 0,
      title: null,
      updated_at: "2026-11-21T10:02:00Z",
    },
  } as ObserveRequest

  it.each([
    ["a new revision", { status: 202, body: { request: revision } }, true],
    [
      "a replay of the first one",
      { status: 200, body: { request: revision, replayed: true } },
      true,
    ],
    [
      "a change already applied",
      {
        status: 409,
        body: { error: "already applied", code: "already_applied" },
      },
      false,
    ],
  ])(
    "says the revision was asked for, in the same chat, after %s",
    async (_name, answer, joins) => {
      const chat = {
        ...chatAbout({ dashboard_id: dashboardId }),
        turns: [answered()],
      }
      stubServer(chat, answer, [answered(), revision])
      const { user, router } = renderChatPage(answered())
      await user.click(
        await screen.findByRole("button", { name: "Revise the dashboard" })
      )
      expect(
        await screen.findByRole("button", { name: "Revision asked for" })
      ).toHaveAttribute("aria-disabled", "true")
      expect(router?.state.location.pathname).toBe(`/c/${rootId}`)
      if (joins) {
        expect(
          await screen.findByText("Add a p99 panel", { exact: true })
        ).toBeVisible()
      }
    }
  )

  it("keeps Revise the dashboard when the revision is refused", async () => {
    stubServer(undefined, {
      status: 429,
      body: { error: "too many revisions", code: "too_many_revisions" },
    })
    const { user } = renderConversation(answered())
    const button = await screen.findByRole("button", {
      name: "Revise the dashboard",
    })
    await user.click(button)
    await waitFor(() =>
      expect(button).not.toHaveAttribute("aria-disabled", "true")
    )
    expect(
      screen.queryByRole("button", { name: "Revision asked for" })
    ).toBeNull()
  })

  it("holds Revise the dashboard while no model is connected", async () => {
    const fetch = stubServer()
    const { queryClient } = renderConversation(answered())
    queryClient.setQueryData(observeKeys.agent(), { available: false })
    const button = await screen.findByRole("button", {
      name: "Revise the dashboard",
    })
    await waitFor(() => expect(button).toHaveAttribute("aria-disabled", "true"))
    expect(button).toHaveAccessibleDescription(noModelReviseText)
    expect(
      fetch.mock.calls.some(([request]) => request.method === "POST")
    ).toBe(false)
  })

  it("keeps the finding card, with no Revise, for an investigation", async () => {
    stubServer()
    renderConversation(
      answered({
        investigation: true,
        observation_id: observationId,
        result_kind: "observation",
        finding: { ...finding, observation_id: observationId },
      })
    )
    expect(await screen.findByText(/^Found/)).toBeVisible()
    expect(
      screen.queryByRole("button", { name: "Revise the dashboard" })
    ).toBeNull()
  })

  it("offers no Revise on an answer whose turn has no dashboard, in a chat about one", async () => {
    stubServer(chatAbout({ dashboard_id: dashboardId }))
    renderConversation(answered({ dashboard_id: null }))
    expect(
      await screen.findByText("Suggested change: Add a p99 panel")
    ).toBeVisible()
    expect(
      screen.queryByRole("button", { name: "Revise the dashboard" })
    ).toBeNull()
  })

  it("shows the load error, with no composer, when the chat cannot be read", async () => {
    const fetch = stubServer(null)
    const { user } = renderConversation()
    expect(await screen.findByText("Could not load this request")).toBeVisible()
    expect(
      screen.queryByRole("textbox", { name: "Message the agent" })
    ).toBeNull()
    const reads = () =>
      fetch.mock.calls.filter(
        ([request]) =>
          new URL(request.url).pathname === `/api/v1/chats/${rootId}`
      ).length
    const before = reads()
    await user.click(screen.getByRole("button", { name: "Retry" }))
    await waitFor(() => expect(reads()).toBe(before + 1))
  })

  it("shows a run from someone else's private chat without its words or actions", async () => {
    stubServer(null)
    const { user } = renderConversation(
      redactedRun({
        id: rootId,
        status: "failed",
        state: "failed",
        finished_at: "2026-11-21T10:01:00Z",
        error: "the agent stopped",
        failure_code: "no_progress",
        steps: [
          {
            at: "2026-11-21T10:00:30Z",
            attempt: 0,
            phase: "discover",
            tool: "list_metrics",
            status: "ok",
            message: "",
            duration_ms: 10,
          },
        ],
      })
    )
    expect(
      await screen.findByText("Asked in a private chat", { selector: "div" })
    ).toBeVisible()
    await waitFor(() =>
      expect(document.title).toBe("Asked in a private chat - SlashObserve")
    )
    await user.click(await screen.findByText(/^Ran 1 step/))
    expect(screen.getByText("list_metrics")).toBeVisible()
    expect(screen.queryByRole("button", { name: "Copy message" })).toBeNull()
    expect(screen.queryByRole("button", { name: /Try again/ })).toBeNull()
    expect(screen.queryByRole("textbox")).toBeNull()
  })

  it("lets an Admin read another person's chat in full with no actions", async () => {
    stubServer(chatAbout({ dashboard_id: dashboardId }))
    const { user } = renderConversation(
      answered({
        read_only: true,
        requested_by: { kind: "user", id: "u2", name: "Grace" },
      })
    )
    expect(await screen.findByText("p99 rose after the deploy.")).toBeVisible()
    expect(screen.getByText("Suggested change: Add a p99 panel")).toBeVisible()
    expect(
      screen.queryByRole("textbox", { name: "Message the agent" })
    ).toBeNull()
    expect(screen.queryByRole("button", { name: "Ask again" })).toBeNull()
    expect(
      screen.queryByRole("button", { name: "Revise the dashboard" })
    ).toBeNull()
    await openAnswerMenu(user)
    expect(
      await screen.findByRole("menuitem", { name: /^Copy answer/ })
    ).toBeVisible()
    expect(
      screen.queryByRole("menuitem", { name: /^Show to everyone/ })
    ).toBeNull()
    expect(screen.queryByRole("menuitem", { name: /^Withdraw/ })).toBeNull()
  })

  it("puts Ask again in the answer's own row of actions, before the menu", async () => {
    stubServer(chatAbout({ dashboard_id: dashboardId }))
    renderConversation(answered({ kind: "build" }))
    const askAgain = await screen.findByRole("button", { name: "Ask again" })
    const menu = screen.getByRole("button", { name: "Answer actions" })
    const row = askAgain.parentElement
    expect(row).toBe(menu.parentElement)
    const children = Array.from(row!.children)
    expect(children.indexOf(askAgain)).toBeLessThan(children.indexOf(menu))
    expect(row!.parentElement).toContainElement(
      screen.getByText("p99 rose after the deploy.")
    )
  })

  it("suggests two follow-ups in the side panel of a chat that nothing was asked in yet, and asks one there", async () => {
    const fetch = stubServer(chatAbout({ dashboard_id: dashboardId }))
    const { user } = renderConversation(answered(), null, "pane")
    const group = await screen.findByRole("group", {
      name: "Suggested follow-ups",
    })
    const chips = Array.from(group.querySelectorAll("button"))
    expect(chips.map((chip) => chip.textContent)).toEqual([
      "Which services had the most errors in the last hour?",
      "Which routes are slowest right now?",
    ])
    await user.click(chips[1])
    const posted = await vi.waitFor(() => {
      const found = fetch.mock.calls.find(
        ([request]) =>
          request.method === "POST" &&
          new URL(request.url).pathname === "/api/v1/observe"
      )
      expect(found).toBeDefined()
      return found![0]
    })
    expect(await posted.clone().json()).toMatchObject({
      prompt: "Which routes are slowest right now?",
      kind: "ask",
      dashboard_id: dashboardId,
      chat_id: rootId,
    })
  })

  it("suggests none on the chat page", async () => {
    stubServer(chatAbout({ dashboard_id: dashboardId }))
    renderConversation(answered())
    await screen.findByText("p99 rose after the deploy.")
    expect(
      screen.queryByRole("group", { name: "Suggested follow-ups" })
    ).toBeNull()
  })
})
