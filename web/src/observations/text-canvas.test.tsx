import { fireEvent, screen, waitFor, within } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { observeKeys, threadKeys } from "@/api/keys"
import type { Message, Thread } from "@/comments/thread-queries"
import { useCanvasTool } from "@/dashboards/canvas-tools"
import { renderWithProviders, testQueryClient } from "@/test/render"

import { BlockList } from "./block-list"
import type { Block, Observation } from "./queries"

vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  useRouteContext: () => ({ principal: { id: "u1", role: "editor" } }),
}))

const author = {
  kind: "user" as const,
  id: "u1",
  name: "Ada",
  origin: "web" as const,
  client: null,
  run_id: null,
  label: "Ada",
}

const textBlock = {
  id: "b1",
  observation_id: "o1",
  kind: "text",
  state: "live",
  position: 0,
  width: "full",
  height: null,
  version: 1,
  body: { markdown: "Checkout errors rose after the deploy." },
  snapshot_id: null,
  run_id: null,
  created_by: author,
  created_at: "2026-11-12T10:00:00Z",
  updated_at: "2026-11-12T10:00:00Z",
  allowed_actions: ["update"],
} as Block

const observation = {
  id: "o1",
  allowed_actions: [],
} as unknown as Observation

function thread(id: string, quote: string): Thread {
  return {
    id,
    target: { kind: "observation", id: "o1" },
    dashboard_id: null,
    version: null,
    panel_id: null,
    panel_present: true,
    status: "open",
    allowed_actions: ["reply"],
    unread: false,
    created_at: "2026-11-12T10:00:00Z",
    updated_at: "2026-11-12T10:00:00Z",
    resolved_at: null,
    resolved_actor: null,
    resolved_by_version: null,
    revision: null,
    anchor: { kind: "text", pos: { x: 0.1, y: 0.5 }, quote },
    block_id: "b1",
    agent: { kind: "builtin", id: null },
    agent_off: false,
  } as Thread
}

function message(threadId: string): Message {
  return {
    id: `${threadId}-m`,
    thread_id: threadId,
    body: "Which deploy?",
    actor: author,
    created_at: "2026-11-12T10:00:00Z",
    updated_at: "2026-11-12T10:00:00Z",
    edited_at: null,
    mentions: [],
    can_edit: false,
    can_delete: false,
    block_id: "b1",
    reply_to: null,
  } as Message
}

function show({
  canComment = true,
  threads = [] as Thread[],
}: { canComment?: boolean; threads?: Thread[] } = {}) {
  const queryClient = testQueryClient()
  queryClient.setQueryData(observeKeys.agent(), {
    available: true,
    provider: "anthropic",
    model: "claude-haiku-5-5",
    effort: null,
  })
  queryClient.setQueryData(threadKeys.list("observation:o1"), {
    pages: [
      {
        threads,
        messages: threads.map((each) => message(each.id)),
        next_cursor: null,
        can_comment: true,
      },
    ],
    pageParams: [undefined],
  })
  return renderWithProviders(
    <BlockList
      observation={observation}
      blocks={[textBlock]}
      canAdd
      canComment={canComment}
      reveal={null}
      onRevealed={() => {}}
      onAdded={() => {}}
    />,
    { queryClient, route: { path: "/", url: "/" } }
  )
}

function selectText(text: string) {
  const node = screen.getByText(text)
  const selection = document.getSelection()
  selection?.selectAllChildren(node)
  fireEvent.pointerUp(node)
}

beforeEach(() => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => Response.json({ requests: [], next_cursor: null }))
  )
  vi.spyOn(Range.prototype, "getClientRects").mockReturnValue([
    DOMRect.fromRect({ x: 10, y: 20, width: 120, height: 16 }),
  ] as unknown as DOMRectList)
  vi.spyOn(HTMLElement.prototype, "offsetWidth", "get").mockReturnValue(400)
  vi.spyOn(HTMLElement.prototype, "offsetHeight", "get").mockReturnValue(100)
  useCanvasTool.setState({
    tool: "select",
    draft: null,
    openThreadId: null,
    openTurn: null,
    postedThreadId: null,
    text: "",
  })
})

afterEach(() => {
  document.getSelection()?.removeAllRanges()
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

describe("a text block on the canvas", () => {
  it("offers Comment and Ask on selected text and opens a draft on the quote", async () => {
    const { user } = show()
    await screen.findByText("Checkout errors rose after the deploy.")
    selectText("Checkout errors rose after the deploy.")
    const actions = await screen.findByRole("toolbar", {
      name: "Selected text",
    })
    expect(
      within(actions).getByRole("button", { name: "Ask" })
    ).toBeInTheDocument()
    await user.click(within(actions).getByRole("button", { name: "Comment" }))
    expect(useCanvasTool.getState().draft).toMatchObject({
      ownerId: "b1",
      tool: "comment",
      anchor: {
        kind: "text",
        quote: "Checkout errors rose after the deploy.",
      },
    })
    expect(
      await screen.findByRole("dialog", { name: "Comment on the canvas" })
    ).toBeInTheDocument()
  })

  it("opens the draft of the chosen mode straight from a selection", async () => {
    useCanvasTool.setState({ tool: "ask" })
    show()
    await screen.findByText("Checkout errors rose after the deploy.")
    selectText("Checkout errors rose after the deploy.")
    await waitFor(() =>
      expect(useCanvasTool.getState().draft).toMatchObject({
        ownerId: "b1",
        tool: "ask",
      })
    )
    expect(
      screen.queryByRole("toolbar", { name: "Selected text" })
    ).not.toBeInTheDocument()
  })

  it("offers nothing to someone who cannot comment", async () => {
    show({ canComment: false })
    await screen.findByText("Checkout errors rose after the deploy.")
    selectText("Checkout errors rose after the deploy.")
    expect(
      screen.queryByRole("toolbar", { name: "Selected text" })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole("toolbar", { name: "Canvas tools" })
    ).not.toBeInTheDocument()
  })

  it("pins a thread at its text and sets one apart when the text is gone", async () => {
    show({
      threads: [
        thread("t1", "errors rose"),
        thread("t2", "latency fell overnight"),
      ],
    })
    const found = await screen.findByRole("button", {
      name: "Comment by Ada: Which deploy?",
    })
    expect(found).not.toHaveAttribute("data-gone")
    const gone = screen.getByRole("button", {
      name: "Comment by Ada: Which deploy?, its text is no longer in the block",
    })
    expect(gone).toHaveAttribute("data-gone")
  })

  it("enters and leaves the comment mode with C and Esc", async () => {
    const { user } = show()
    await screen.findByRole("toolbar", { name: "Canvas tools" })
    await user.keyboard("c")
    expect(useCanvasTool.getState().tool).toBe("comment")
    expect(screen.getByRole("status")).toHaveTextContent(
      "select text, to comment on it"
    )
    await user.keyboard("{Escape}")
    expect(useCanvasTool.getState().tool).toBe("select")
  })
})
