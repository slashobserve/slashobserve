import { act, screen, waitFor, within } from "@testing-library/react"
import { useRef } from "react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { chatKeys, observeKeys } from "@/api/keys"
import {
  layoutEditingText,
  pinnedText,
  ungated,
  type GateInput,
} from "@/conversation/delivery"
import { useCanvasTool, type OpenTurn } from "@/dashboards/canvas-tools"
import type { Finding } from "@/findings/queries"
import type { ObserveRequest } from "@/observe/queries"
import type { Anchor } from "@/panels/anchors"
import { renderWithProviders, testQueryClient } from "@/test/render"

import type { CanvasSubject } from "./canvas-host"
import {
  AskPopover,
  noModelAskText,
  openChatOf,
  privateText,
  reviseSharedText,
  type AskSpot,
} from "./ask-popover"

vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  useRouteContext: () => ({ principal: { id: "u1", role: "editor" } }),
}))

const dashboardId = "2f0f2c1e-6a53-4a35-8a0e-1d3b0d1f7a11"

const onRow: Anchor = {
  kind: "row",
  pos: { x: 0.4, y: 0.6 },
  row: { service: "payments" },
  shown: "440",
  window: { from: "now-1h", to: "now" },
}

const spot: AskSpot = {
  ownerId: "p1",
  anchor: onRow,
  at: { x: 120, y: 80 },
  surface: "s1",
}

const working = {
  id: "r2",
  chat_id: "c1",
  prompt: "Why is payments slow?",
  status: "running",
  state: "running",
  intent: "ask",
  kind: "investigate",
  dashboard_id: dashboardId,
  panel_id: "p1",
  observation_id: null,
  read_only: false,
  allowed_actions: [],
  requested_by: { kind: "user", id: "u1", name: "Ada" },
  created_at: "2026-11-21T14:31:00Z",
  finding: null,
  steps: [],
  shared_thread_id: null,
} as Partial<ObserveRequest> as ObserveRequest

const finding: Finding = {
  id: "f1",
  observation_id: null,
  agent_request_id: "r2",
  dashboard_id: dashboardId,
  folder_id: null,
  ask_on: null,
  title: "Payments waits on the card network",
  markdown: "Payments waits on the card network.",
  evidence: [],
  from: "2026-11-21T13:31:00Z",
  to: "2026-11-21T14:31:00Z",
  annotation_id: null,
  created_by: { kind: "user", id: "u1", name: "Ada" },
  actor: {
    kind: "user",
    id: "u1",
    name: "Ada",
    origin: "web",
    client: null,
    run_id: "r2",
    label: "Observe Agent for Ada",
  },
  created_at: "2026-11-21T14:32:00Z",
  url: null,
  suggested_change: "Add a card network latency panel",
  observe_worthy: false,
  restricted: false,
}

const answered = {
  ...working,
  status: "succeeded",
  state: "succeeded",
  result_kind: "answer",
  finding,
} as ObserveRequest

function chatOf(turns: ObserveRequest[]) {
  return {
    chat: {
      id: "c1",
      title: "Why is payments slow?",
      dashboard_id: dashboardId,
      observation_id: null,
      pinned_at: null,
      created_at: "2026-11-21T14:31:00Z",
      last_turn_at: "2026-11-21T14:31:00Z",
      turns: turns.length,
      last_turn: null,
      owner: { kind: "user", name: "Ada" },
      read_only: false,
    },
    turns,
  }
}

function stubServer(turns: ObserveRequest[] = []) {
  const posts: Record<string, unknown>[] = []
  const fetch = vi.fn<(request: Request) => Promise<Response>>(
    async (request) => {
      const { pathname } = new URL(request.url)
      if (request.method === "POST" && pathname === "/api/v1/observe") {
        const body = (await request.clone().json()) as Record<string, unknown>
        posts.push(body)
        return Response.json(
          {
            request: {
              ...working,
              id: `q${posts.length}`,
              chat_id: body.chat_id ?? `q${posts.length}`,
              prompt: body.prompt,
            },
          },
          { status: 202 }
        )
      }
      if (pathname === "/api/v1/chats/c1") return Response.json(chatOf(turns))
      if (pathname.startsWith("/api/v1/chats/")) {
        return Response.json(
          { error: { code: "not_found", message: "Not found" } },
          { status: 404 }
        )
      }
      const known = turns.find(({ id }) => pathname === `/api/v1/observe/${id}`)
      if (known !== undefined) return Response.json({ request: known })
      return Response.json({ service_accounts: [], connected: [] })
    }
  )
  vi.stubGlobal("fetch", fetch)
  return posts
}

const onDashboard: CanvasSubject = { kind: "dashboard", id: dashboardId }
const onObservation: CanvasSubject = { kind: "observation", id: "o1" }

function Harness({
  gate,
  openChatId,
  onChatStarted,
  subject,
}: {
  gate: GateInput
  openChatId: string | null
  onChatStarted: (chatId: string) => void
  subject: CanvasSubject
}) {
  const turn = useCanvasTool((state) => state.openTurn)
  const draft = useCanvasTool((state) => state.draft)
  const at = turn ?? draft
  const surface = useRef<HTMLDivElement>(null)
  return (
    <>
      <button type="button">Ask origin</button>
      <div role="toolbar">
        <button type="button">Ask tool</button>
      </div>
      <article>
        <button type="button" data-panel-title="">
          Slow services title
        </button>
        <div ref={surface} />
      </article>
      {at === null ? null : (
        <AskPopover
          subject={subject}
          panelTitle="Slow services"
          surfaceRef={surface}
          gate={gate}
          spot={{
            ownerId: at.ownerId,
            anchor: at.anchor,
            at: at.at,
            surface: at.surface,
          }}
          turn={turn}
          openChatId={openChatId}
          onChatStarted={onChatStarted}
        />
      )}
    </>
  )
}

function seeded(agentAvailable = true) {
  const queryClient = testQueryClient()
  queryClient.setQueryData(observeKeys.agent(), {
    available: agentAvailable,
    provider: "anthropic",
    model: "claude-haiku-5-5",
    effort: "medium",
  })
  return queryClient
}

function renderAsk({
  gate = { ...ungated, agentAvailable: true },
  openChatId = null,
  onChatStarted = vi.fn(),
  queryClient = seeded(),
  subject = onDashboard,
}: {
  gate?: GateInput
  openChatId?: string | null
  onChatStarted?: (chatId: string) => void
  queryClient?: ReturnType<typeof testQueryClient>
  subject?: CanvasSubject
} = {}) {
  return renderWithProviders(
    <Harness
      gate={gate}
      openChatId={openChatId}
      onChatStarted={onChatStarted}
      subject={subject}
    />,
    { queryClient, route: { path: "/", url: "/" } }
  )
}

function openDraft() {
  useCanvasTool.setState({
    tool: "ask",
    draft: { tool: "ask", ...spot },
    openTurn: null,
  })
}

function openTurn(turn: Partial<OpenTurn> = {}) {
  useCanvasTool.setState({
    tool: "ask",
    draft: null,
    openTurn: { requestId: "r2", chatId: "c1", ...spot, ...turn },
  })
}

beforeEach(() => {
  useCanvasTool.setState({
    tool: "select",
    draft: null,
    openThreadId: null,
    postedThreadId: null,
    openTurn: null,
    text: "",
    leaving: null,
    detached: null,
  })
})

afterEach(() => vi.unstubAllGlobals())

describe("openChatOf", () => {
  it("reads the chat open in the Chat tab, and none for a new chat", () => {
    expect(openChatOf("c1")).toBe("c1")
    expect(openChatOf("new")).toBeNull()
    expect(openChatOf(undefined)).toBeNull()
  })
})

describe("AskPopover", () => {
  it("opens at the click with the spot's chips, no Visible to chip, and the private note", async () => {
    stubServer()
    openDraft()
    renderAsk()
    const popup = await screen.findByRole("dialog", {
      name: "Ask about Slow services · Row payments · 440 · last hour",
    })
    expect(popup).toHaveAttribute("data-opened-by", "pointer")
    const chips = within(popup).getByRole("list", { name: "Context" })
    for (const label of ["Slow services", "Row payments", "440"]) {
      expect(chips).toHaveTextContent(label)
    }
    expect(within(popup).queryByText(/Visible to/)).toBeNull()
    expect(within(popup).getByText(privateText)).toBeVisible()
    await waitFor(() =>
      expect(
        within(popup).getByRole("textbox", { name: "Ask about this" })
      ).toHaveFocus()
    )
  })

  it("asks privately in the open chat with the spot, the value shown and the window, then shows it working", async () => {
    const posts = stubServer()
    const onChatStarted = vi.fn()
    const queryClient = seeded()
    queryClient.setQueryData(chatKeys.detail("c1"), chatOf([]))
    openDraft()
    const { user } = renderAsk({ openChatId: "c1", onChatStarted, queryClient })
    await user.type(
      await screen.findByRole("textbox", { name: "Ask about this" }),
      "Why is payments slow?{Enter}"
    )
    await waitFor(() => expect(posts).toHaveLength(1))
    expect(posts[0]).toEqual({
      prompt: "Why is payments slow?",
      dashboard_id: dashboardId,
      kind: "ask",
      chat_id: "c1",
      panel_id: "p1",
      anchor: onRow,
      client_id: expect.any(String),
      time_zone: Intl.DateTimeFormat().resolvedOptions().timeZone,
    })
    expect(
      await screen.findByRole("list", { name: "Your chat on this spot" })
    ).toHaveTextContent("Why is payments slow?")
    expect(await screen.findByText("Looking into it")).toBeVisible()
    await waitFor(() =>
      expect(
        screen.getByRole("textbox", { name: "Ask a follow-up" })
      ).toHaveFocus()
    )
    expect(useCanvasTool.getState().openTurn).toMatchObject({
      requestId: "q1",
      chatId: "c1",
      ownerId: "p1",
      anchor: onRow,
    })
    expect(onChatStarted).not.toHaveBeenCalled()
  })

  it("starts a new chat when none is open and selects it in the Chat tab", async () => {
    const posts = stubServer()
    const onChatStarted = vi.fn()
    openDraft()
    const { user } = renderAsk({ onChatStarted })
    await user.type(
      await screen.findByRole("textbox", { name: "Ask about this" }),
      "Why?{Enter}"
    )
    await waitFor(() => expect(onChatStarted).toHaveBeenCalledWith("q1"))
    expect(posts[0]).not.toHaveProperty("chat_id")
  })

  it.each([
    ["an Admin reads someone else's", "c1", { read_only: true }],
    [
      "is about another dashboard",
      "c1",
      { dashboard_id: "7d1c9a40-0000-4000-8000-000000000000" },
    ],
    ["is archived, unknown or a turn's id", "c9", null],
  ])(
    "starts a new chat when the open chat %s",
    async (_, openChatId, change) => {
      const posts = stubServer()
      const queryClient = seeded()
      if (change !== null) {
        const open = chatOf([])
        queryClient.setQueryData(chatKeys.detail("c1"), {
          ...open,
          chat: { ...open.chat, ...change },
        })
      }
      const onChatStarted = vi.fn()
      openDraft()
      const { user } = renderAsk({ openChatId, onChatStarted, queryClient })
      const field = await screen.findByRole("textbox", {
        name: "Ask about this",
      })
      await user.type(field, "Why?{Enter}")
      await waitFor(() => expect(onChatStarted).toHaveBeenCalledWith("q1"))
      expect(posts[0]).not.toHaveProperty("chat_id")
    }
  )

  it("swaps the working state for the answer with Revise and the answer menu", async () => {
    stubServer([working])
    const queryClient = seeded()
    queryClient.setQueryData(observeKeys.request("r2"), working)
    openTurn()
    const { user } = renderAsk({ queryClient })
    expect(await screen.findByText("Looking into it")).toBeVisible()
    queryClient.setQueryData(observeKeys.request("r2"), answered)
    expect(
      await screen.findByText("Payments waits on the card network.")
    ).toBeVisible()
    expect(screen.queryByText("Looking into it")).toBeNull()
    const revise = screen.getByRole("button", { name: "Revise the dashboard" })
    expect(revise).toBeVisible()
    expect(revise).toHaveAccessibleDescription(reviseSharedText)
    expect(screen.getByText(reviseSharedText)).toBeVisible()
    await user.click(screen.getByRole("button", { name: "Answer actions" }))
    expect(
      await screen.findByRole("menuitem", { name: /^Share to thread/ })
    ).toBeVisible()
    expect(
      screen.queryByRole("menuitem", { name: /^Show to everyone/ })
    ).toBeNull()
  })

  it("reopens an answered turn from its pin, named for the spot", async () => {
    stubServer([answered])
    const queryClient = seeded()
    queryClient.setQueryData(observeKeys.request("r2"), answered)
    openTurn()
    renderAsk({ queryClient })
    const popup = await screen.findByRole("dialog", {
      name: "Your question on Slow services, Row payments, 440, last hour",
    })
    expect(
      within(popup).getByText("Payments waits on the card network.")
    ).toBeVisible()
    expect(
      within(popup).getByRole("textbox", { name: "Ask a follow-up" })
    ).toBeVisible()
  })

  it("asks a follow-up in the same chat on the same spot and lists it under the first", async () => {
    const posts = stubServer([answered])
    const queryClient = seeded()
    queryClient.setQueryData(observeKeys.request("r2"), answered)
    queryClient.setQueryData(chatKeys.detail("c1"), chatOf([answered]))
    openTurn()
    const { user } = renderAsk({ queryClient })
    await user.type(
      await screen.findByRole("textbox", { name: "Ask a follow-up" }),
      "And yesterday?{Enter}"
    )
    await waitFor(() => expect(posts).toHaveLength(1))
    expect(posts[0]).toMatchObject({
      prompt: "And yesterday?",
      chat_id: "c1",
      panel_id: "p1",
      anchor: onRow,
    })
    const turns = screen.getByRole("list", { name: "Your chat on this spot" })
    await waitFor(() =>
      expect(within(turns).getAllByRole("listitem")).toHaveLength(2)
    )
    expect(within(turns).getAllByRole("listitem")[1]).toHaveTextContent(
      "And yesterday?"
    )
  })

  it.each([
    ["continues the open chat about this observation", "o1", "c1"],
    [
      "starts a new chat when the open one is about another observation",
      "o2",
      undefined,
    ],
  ])("on an observation block, %s", async (_name, about, chatId) => {
    const posts = stubServer()
    const queryClient = seeded()
    const chat = chatOf([])
    queryClient.setQueryData(chatKeys.detail("c1"), {
      ...chat,
      chat: { ...chat.chat, dashboard_id: null, observation_id: about },
    })
    useCanvasTool.setState({
      tool: "ask",
      draft: { tool: "ask", ...spot, ownerId: "b1" },
      openTurn: null,
    })
    const { user } = renderAsk({
      openChatId: "c1",
      queryClient,
      subject: onObservation,
    })
    await user.type(
      await screen.findByRole("textbox", { name: "Ask about this" }),
      "Why is payments slow?{Enter}"
    )
    await waitFor(() => expect(posts).toHaveLength(1))
    expect(posts[0]).toMatchObject({
      observation_id: "o1",
      kind: "ask",
      block_id: "b1",
      anchor: onRow,
    })
    expect(posts[0]).not.toHaveProperty("panel_id")
    expect(posts[0].chat_id).toBe(chatId)
  })

  it("lists the turns asked at the same spot of the same observation block", async () => {
    const onBlock = (id: string, blockId: string, prompt: string) =>
      ({
        ...answered,
        id,
        prompt,
        dashboard_id: null,
        panel_id: null,
        block_id: blockId,
        anchor: onRow,
        retried_by: null,
      }) as ObserveRequest
    const turns = [
      onBlock("r2", "b1", "Why is payments slow?"),
      onBlock("r3", "b1", "And yesterday?"),
      onBlock("r4", "b2", "And on the other block?"),
    ]
    stubServer(turns)
    const queryClient = seeded()
    for (const turn of turns) {
      queryClient.setQueryData(observeKeys.request(turn.id), turn)
    }
    queryClient.setQueryData(chatKeys.detail("c1"), chatOf(turns))
    openTurn({ ownerId: "b1" })
    renderAsk({ queryClient, subject: onObservation })
    const listed = await screen.findByRole("list", {
      name: "Your chat on this spot",
    })
    await waitFor(() =>
      expect(within(listed).getAllByRole("listitem")).toHaveLength(2)
    )
    expect(listed).toHaveTextContent("And yesterday?")
    expect(listed).not.toHaveTextContent("And on the other block?")
  })

  it("closes on Close and clears the open turn", async () => {
    stubServer([answered])
    const queryClient = seeded()
    queryClient.setQueryData(observeKeys.request("r2"), answered)
    openTurn()
    const { user } = renderAsk({ queryClient })
    await user.click(await screen.findByRole("button", { name: "Close" }))
    expect(useCanvasTool.getState().openTurn).toBeNull()
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull())
  })

  it("returns focus to where it was opened from when it closes", async () => {
    stubServer()
    const { user } = renderAsk()
    const origin = await screen.findByRole("button", { name: "Ask origin" })
    origin.focus()
    act(() => openDraft())
    await waitFor(() =>
      expect(
        screen.getByRole("textbox", { name: "Ask about this" })
      ).toHaveFocus()
    )
    await user.keyboard("{Escape}")
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull())
    expect(useCanvasTool.getState().draft).toBeNull()
    expect(origin).toHaveFocus()
  })

  it("returns focus to the panel title, not the toolbar, when it was opened from the toolbar", async () => {
    stubServer()
    const { user } = renderAsk()
    const tool = await screen.findByRole("button", { name: "Ask tool" })
    tool.focus()
    act(() => openDraft())
    await waitFor(() =>
      expect(
        screen.getByRole("textbox", { name: "Ask about this" })
      ).toHaveFocus()
    )
    await user.keyboard("{Escape}")
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull())
    expect(
      screen.getByRole("button", { name: "Slow services title" })
    ).toHaveFocus()
  })

  it("stays open with a typed question on a click outside, and closes when empty", async () => {
    stubServer()
    const { user } = renderAsk()
    act(() => openDraft())
    const field = await screen.findByRole("textbox", { name: "Ask about this" })
    await user.type(field, "why the spike")
    await user.click(document.body)
    expect(field).toHaveValue("why the spike")
    expect(useCanvasTool.getState().draft).not.toBeNull()
    await user.clear(field)
    await user.click(document.body)
    await waitFor(() => expect(useCanvasTool.getState().draft).toBeNull())
  })

  it("keeps a typed follow-up when another spot or pin is opened", async () => {
    stubServer([answered])
    const queryClient = seeded()
    queryClient.setQueryData(observeKeys.request("r2"), answered)
    openTurn()
    const { user } = renderAsk({ queryClient })
    const field = await screen.findByRole("textbox", {
      name: "Ask a follow-up",
    })
    await user.type(field, "and yesterday")
    act(() => {
      useCanvasTool.getState().openDraft({ tool: "ask", ...spot })
      useCanvasTool
        .getState()
        .showTurn({ requestId: "r9", chatId: "c2", ...spot })
    })
    expect(useCanvasTool.getState().openTurn).toMatchObject({
      requestId: "r2",
    })
    expect(field).toHaveValue("and yesterday")
  })

  it("leaves the box closed and a newer draft alone when a slow send returns after it closed", async () => {
    const posts = stubServer()
    const served = globalThis.fetch
    let release = () => {}
    const held = new Promise<void>((resolve) => {
      release = resolve
    })
    vi.stubGlobal(
      "fetch",
      vi.fn(async (request: Request) => {
        if (request.method === "POST") await held
        return served(request)
      })
    )
    const onChatStarted = vi.fn()
    const { user } = renderAsk({ onChatStarted })
    act(() => openDraft())
    await user.type(
      await screen.findByRole("textbox", { name: "Ask about this" }),
      "why the spike{Enter}"
    )
    const comment = { ...spot, tool: "comment" as const, at: { x: 9, y: 9 } }
    act(() => {
      useCanvasTool.getState().close()
      useCanvasTool.getState().openDraft(comment)
    })
    release()
    await waitFor(() => expect(posts).toHaveLength(1))
    await act(async () => {})
    expect(useCanvasTool.getState()).toMatchObject({
      draft: comment,
      openTurn: null,
    })
    expect(onChatStarted).not.toHaveBeenCalled()
  })

  it("says why it cannot ask when no model is connected", async () => {
    const posts = stubServer()
    openDraft()
    renderAsk({
      gate: { ...ungated, agentAvailable: false },
      queryClient: seeded(false),
    })
    expect(await screen.findByText(noModelAskText)).toBeVisible()
    expect(
      screen.getByRole("textbox", { name: "Ask about this" })
    ).toBeDisabled()
    expect(posts).toHaveLength(0)
  })

  it.each([
    ["an older version is pinned", { pinned: true }, pinnedText],
    ["the layout is being edited", { layoutEditing: true }, layoutEditingText],
  ])("holds the question while %s", async (_case, patch, reason) => {
    const posts = stubServer()
    openDraft()
    const { user } = renderAsk({
      gate: { ...ungated, agentAvailable: true, ...patch },
    })
    await user.type(
      await screen.findByRole("textbox", { name: "Ask about this" }),
      "Why?{Enter}"
    )
    expect((await screen.findAllByText(reason))[0]).toBeVisible()
    expect(posts).toHaveLength(0)
  })
})
