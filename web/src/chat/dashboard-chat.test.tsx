import { useState } from "react"
import { cleanup, screen, waitFor } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import type { Chat } from "./chats-queries"
import type { ChatPanel } from "./panel-target"
import { DashboardChat } from "./dashboard-chat"

const dashboardId = "11111111-1111-4111-8111-111111111111"
const otherDashboardId = "22222222-2222-4222-8222-222222222222"

vi.mock("./conversation", () => ({
  Conversation: (props: {
    requestId: string
    asks?: boolean
    panel?: { id: string } | null
  }) => (
    <div
      data-testid="conversation"
      data-request={props.requestId}
      data-asks={String(props.asks ?? true)}
      data-panel={props.panel?.id ?? ""}
    />
  ),
}))

vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  useRouteContext: () => ({ principal: { id: "u1", role: "editor" } }),
}))

function composerBox() {
  return screen.queryByRole("textbox", { name: "Message the agent" })
}

function chat(id: string, title: string, over: Partial<Chat> = {}): Chat {
  return {
    id,
    title,
    dashboard_id: dashboardId,
    dashboard_title: "Checkout health",
    observation_id: null,
    pinned_at: null,
    created_at: "2026-11-20T10:00:00Z",
    last_turn_at: "2026-11-20T10:05:00Z",
    turns: 2,
    last_turn: {
      id: `${id}-t2`,
      state: "succeeded",
      prompt: "x",
      investigation: false,
      subject_observation_id: null,
      anchor: null,
      block_id: null,
    },
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

function render(
  chatId: string | undefined,
  onOpenChat = vi.fn(),
  panel: ChatPanel | null = null
) {
  const view = renderWithProviders(
    <DashboardChat
      dashboardId={dashboardId}
      dashboardTitle="Checkout health"
      chatId={chatId}
      onOpenChat={onOpenChat}
      panel={panel}
    />,
    { route: { path: "/", url: "/" } }
  )
  return { ...view, onOpenChat }
}

function Address({ initial }: { initial: string | undefined }) {
  const [entries, setEntries] = useState<(string | undefined)[]>([initial])
  const chatId = entries.at(-1)
  return (
    <>
      <button
        type="button"
        onClick={() => setEntries((all) => all.slice(0, -1))}
      >
        Back
      </button>
      <output aria-label="Address">{chatId ?? "none"}</output>
      <output aria-label="Entries">{entries.length}</output>
      <DashboardChat
        dashboardId={dashboardId}
        dashboardTitle="Checkout health"
        chatId={chatId}
        onOpenChat={(next, options) =>
          setEntries((all) =>
            options?.replace === true
              ? [...all.slice(0, -1), next]
              : [...all, next]
          )
        }
      />
    </>
  )
}

function renderAddress(initial: string | undefined) {
  return renderWithProviders(<Address initial={initial} />, {
    route: { path: "/", url: "/" },
  })
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
    const { user } = renderAddress("c1")
    await screen.findByTestId("conversation")
    await user.click(screen.getByRole("button", { name: "New chat" }))
    expect(composerBox()).toHaveAttribute(
      "placeholder",
      "Ask about Checkout health"
    )
    expect(
      screen.getByText("Only you and Admins can see this chat.")
    ).toBeVisible()
    expect(screen.queryByTestId("conversation")).not.toBeInTheDocument()
    expect(screen.getByRole("button", { name: "New chat" })).toBeDisabled()
  })

  it("asks about the panel it was opened on in a new chat and in the open one", async () => {
    stub([chat("c1", "Why did checkout errors rise")])
    const panel = { id: "p1", title: "Latency", onRemove: vi.fn() }
    render("new", vi.fn(), panel)
    const box = await screen.findByRole("textbox", {
      name: "Message the agent",
    })
    await waitFor(() => expect(box).toHaveFocus())
    expect(screen.getByRole("list", { name: "Context" })).toHaveTextContent(
      "Latency"
    )
    cleanup()
    render("c1", vi.fn(), panel)
    expect(await screen.findByTestId("conversation")).toHaveAttribute(
      "data-panel",
      "p1"
    )
  })

  it("holds the chat it opened when the list reorders", async () => {
    const listed = [
      chat("c1", "Why did checkout errors rise"),
      chat("c2", "Card declines by issuer"),
    ]
    stub(listed)
    const { queryClient } = renderAddress(undefined)
    expect((await screen.findByTestId("conversation")).dataset.request).toBe(
      "c1"
    )
    await vi.waitFor(() =>
      expect(screen.getByLabelText("Address")).toHaveTextContent("c1")
    )
    expect(screen.getByLabelText("Entries")).toHaveTextContent("1")
    listed.reverse()
    await queryClient.invalidateQueries()
    expect(screen.getByTestId("conversation").dataset.request).toBe("c1")
  })

  it("goes back to the chat it left when the address goes back", async () => {
    stub([chat("c1", "Why did checkout errors rise")])
    const { user } = renderAddress("c1")
    await screen.findByTestId("conversation")
    await user.click(screen.getByRole("button", { name: "New chat" }))
    expect(composerBox()).toBeInTheDocument()
    await user.click(screen.getByRole("button", { name: "Back" }))
    expect(screen.getByLabelText("Address")).toHaveTextContent("c1")
    expect((await screen.findByTestId("conversation")).dataset.request).toBe(
      "c1"
    )
    expect(composerBox()).not.toBeInTheDocument()
  })

  it("says the chats did not load and loads them again on Retry", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({ error: "boom", code: "internal_error" }),
            { status: 500, headers: { "Content-Type": "application/json" } }
          )
      )
    )
    const { user } = render(undefined)
    expect(
      await screen.findByText("Could not load your chats")
    ).toBeInTheDocument()
    stub([chat("c1", "Why did checkout errors rise")])
    await user.click(screen.getByRole("button", { name: "Retry" }))
    expect(await screen.findByTestId("conversation")).toBeInTheDocument()
  })

  it("replaces a link to a later turn with the turn's chat", async () => {
    const root = chat("c1", "Why did checkout errors rise")
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: Request | string, init?: RequestInit) => {
        const req = input instanceof Request ? input : new Request(input, init)
        const path = new URL(req.url).pathname
        const [status, body] =
          path === "/api/v1/chats/c1"
            ? [200, { chat: root, turns: [] }]
            : path === "/api/v1/chats/c1-t2"
              ? [404, { error: "chat not found", code: "not_found" }]
              : path === "/api/v1/observe/c1-t2"
                ? [200, { request: { id: "c1-t2", chat_id: "c1" } }]
                : [200, { chats: [root], next_cursor: null }]
        return new Response(JSON.stringify(body), {
          status,
          headers: { "Content-Type": "application/json" },
        })
      })
    )
    renderAddress("c1-t2")
    await vi.waitFor(() =>
      expect(screen.getByLabelText("Address")).toHaveTextContent(/^c1$/)
    )
    expect(screen.getByLabelText("Entries")).toHaveTextContent("1")
    expect(screen.getByTestId("conversation").dataset.request).toBe("c1")
    expect(screen.getByRole("combobox", { name: "Chat" })).toHaveTextContent(
      "Why did checkout errors rise"
    )
  })

  it("shows a composer when this dashboard has no chats", async () => {
    stub([])
    render(undefined)
    expect(
      await screen.findByRole("textbox", { name: "Message the agent" })
    ).toBeInTheDocument()
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
