import { screen } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import type { Chat } from "./chats-queries"
import { DashboardChat } from "./dashboard-chat"

const dashboardId = "11111111-1111-4111-8111-111111111111"
const otherDashboardId = "22222222-2222-4222-8222-222222222222"

vi.mock("./conversation", () => ({
  Conversation: (props: { requestId: string; asks?: boolean }) => (
    <div
      data-testid="conversation"
      data-request={props.requestId}
      data-asks={String(props.asks ?? true)}
    />
  ),
}))

vi.mock("./chat-composer", () => ({
  ChatComposer: (props: { dashboardId?: string }) => (
    <div data-testid="composer" data-dashboard={props.dashboardId} />
  ),
}))

function chat(id: string, title: string, over: Partial<Chat> = {}): Chat {
  return {
    id,
    title,
    dashboard_id: dashboardId,
    observation_id: null,
    pinned_at: null,
    created_at: "2026-11-20T10:00:00Z",
    last_turn_at: "2026-11-20T10:05:00Z",
    turns: 2,
    last_turn: { id: `${id}-t2`, state: "succeeded", prompt: "x" },
    owner: { kind: "user", name: "Ada" },
    read_only: false,
    ...over,
  }
}

function stub(listed: Chat[], details: Chat[] = listed) {
  const urls: string[] = []
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request | string, init?: RequestInit) => {
      const req = input instanceof Request ? input : new Request(input, init)
      urls.push(req.url)
      const path = new URL(req.url).pathname
      const detail = details.find((c) => path === `/api/v1/chats/${c.id}`)
      const body = detail
        ? { chat: detail, turns: [] }
        : { chats: listed, next_cursor: null }
      return new Response(JSON.stringify(body), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      })
    })
  )
  return urls
}

function render(chatId: string | undefined, onOpenChat = vi.fn()) {
  const view = renderWithProviders(
    <DashboardChat
      dashboardId={dashboardId}
      dashboardTitle="Checkout health"
      chatId={chatId}
      onOpenChat={onOpenChat}
    />,
    { route: { path: "/", url: "/" } }
  )
  return { ...view, onOpenChat }
}

afterEach(() => vi.unstubAllGlobals())

describe("DashboardChat", () => {
  it("opens the latest chat on this dashboard when none is chosen", async () => {
    const urls = stub([
      chat("c1", "Why did checkout errors rise"),
      chat("c2", "Card declines by issuer"),
    ])
    render(undefined)
    const conversation = await screen.findByTestId("conversation")
    expect(conversation.dataset.request).toBe("c1")
    expect(conversation.dataset.asks).toBe("true")
    expect(
      urls.some((url) => url.includes(`dashboard_id=${dashboardId}`))
    ).toBe(true)
  })

  it("opens the chat chosen in the select", async () => {
    stub([
      chat("c1", "Why did checkout errors rise"),
      chat("c2", "Card declines by issuer"),
    ])
    const { user, onOpenChat } = render("c1")
    await screen.findByTestId("conversation")
    await user.click(screen.getByRole("combobox", { name: "Chat" }))
    await user.click(
      await screen.findByRole("option", { name: /Card declines by issuer/ })
    )
    expect(onOpenChat).toHaveBeenCalledWith("c2")
  })

  it("starts a new chat with a composer for this dashboard", async () => {
    stub([chat("c1", "Why did checkout errors rise")])
    const { user, onOpenChat } = render("c1")
    await screen.findByTestId("conversation")
    await user.click(screen.getByRole("button", { name: "New chat" }))
    expect(onOpenChat).toHaveBeenCalledWith(undefined)
    expect(screen.getByTestId("composer").dataset.dashboard).toBe(dashboardId)
    expect(screen.queryByTestId("conversation")).not.toBeInTheDocument()
    expect(screen.getByRole("button", { name: "New chat" })).toBeDisabled()
  })

  it("shows a composer when this dashboard has no chats", async () => {
    stub([])
    render(undefined)
    expect(await screen.findByTestId("composer")).toBeInTheDocument()
  })

  it("sends a chat about another dashboard to the chat page", async () => {
    const elsewhere = chat("c9", "Disk pressure on node-3", {
      dashboard_id: otherDashboardId,
    })
    stub([chat("c1", "Why did checkout errors rise")], [elsewhere])
    render("c9")
    expect(
      await screen.findByRole("link", { name: "Continue it on the chat page" })
    ).toHaveAttribute("href", "/c/c9")
    expect(screen.getByTestId("conversation").dataset.asks).toBe("false")
  })
})
