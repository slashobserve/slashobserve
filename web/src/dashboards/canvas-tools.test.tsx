import { screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it } from "vitest"

import { observeKeys } from "@/api/keys"
import { renderWithProviders, testQueryClient } from "@/test/render"

import {
  activeTool,
  askModeText,
  CanvasTools,
  commentModeText,
  openKeyDraft,
  useCanvasTool,
  type CanvasDraft,
} from "./canvas-tools"

const draft: CanvasDraft = {
  panelId: "p1",
  tool: "comment",
  anchor: { kind: "position", pos: { x: 0.5, y: 0.5 } },
  at: { x: 40, y: 20 },
  surface: "s1",
}

beforeEach(() => {
  useCanvasTool.setState({
    tool: "select",
    draft: null,
    openThreadId: null,
    openTurn: null,
    postedThreadId: null,
    text: "",
  })
})

describe("useCanvasTool", () => {
  it("keeps a draft that has text when another spot or a thread is opened", () => {
    const other = { ...draft, at: { x: 90, y: 90 } }
    useCanvasTool.getState().openDraft(draft)
    useCanvasTool.getState().setText("half a thought")
    useCanvasTool.getState().openDraft(other)
    useCanvasTool.getState().openThread("t1")
    expect(useCanvasTool.getState()).toMatchObject({
      draft,
      text: "half a thought",
      openThreadId: null,
    })
  })

  it("keeps a draft that has text when a pin's turn is opened", () => {
    useCanvasTool.getState().openDraft(draft)
    useCanvasTool.getState().setText("half a question")
    useCanvasTool
      .getState()
      .showTurn({ requestId: "r1", chatId: "c1", ...draft })
    expect(useCanvasTool.getState()).toMatchObject({
      draft,
      openTurn: null,
      text: "half a question",
    })
  })

  it("keeps an open turn whose follow-up has text, and opens the next once it is empty", () => {
    const turn = { requestId: "r1", chatId: "c1", ...draft }
    useCanvasTool.getState().showTurn(turn)
    useCanvasTool.getState().setText("and yesterday")
    useCanvasTool.getState().openDraft(draft)
    expect(useCanvasTool.getState()).toMatchObject({
      openTurn: turn,
      draft: null,
    })
    useCanvasTool.getState().setText("")
    useCanvasTool.getState().openDraft(draft)
    expect(useCanvasTool.getState()).toMatchObject({ openTurn: null, draft })
  })

  it("drops the text with the draft, and starts the next draft empty", () => {
    useCanvasTool.getState().openDraft(draft)
    useCanvasTool.getState().setText("half a thought")
    useCanvasTool.getState().close()
    expect(useCanvasTool.getState().text).toBe("")
    useCanvasTool.getState().openDraft(draft)
    useCanvasTool.getState().setText("again")
    useCanvasTool.getState().setTool("select")
    expect(useCanvasTool.getState().text).toBe("")
  })

  it("keeps Comment mode on after a draft opens and closes", () => {
    const store = useCanvasTool.getState()
    store.setTool("comment")
    store.openDraft(draft)
    useCanvasTool.getState().close()
    expect(useCanvasTool.getState()).toMatchObject({
      tool: "comment",
      draft: null,
    })
  })

  it("drops the draft and the open thread when the mode changes", () => {
    const store = useCanvasTool.getState()
    store.setTool("comment")
    store.openDraft(draft)
    useCanvasTool.getState().setTool("select")
    expect(useCanvasTool.getState()).toMatchObject({
      tool: "select",
      draft: null,
      openThreadId: null,
    })
  })

  it("holds one draft or one open thread, never both", () => {
    useCanvasTool.getState().openThread("t1")
    useCanvasTool.getState().openDraft(draft)
    expect(useCanvasTool.getState().openThreadId).toBeNull()
    useCanvasTool.getState().openThread("t2")
    expect(useCanvasTool.getState()).toMatchObject({
      draft: null,
      openThreadId: "t2",
    })
  })

  it("marks the posted thread until its pin settles", () => {
    useCanvasTool.getState().openDraft(draft)
    useCanvasTool.getState().posted("t1")
    expect(useCanvasTool.getState()).toMatchObject({
      draft: null,
      postedThreadId: "t1",
    })
    useCanvasTool.getState().settled("t2")
    expect(useCanvasTool.getState().postedThreadId).toBe("t1")
    useCanvasTool.getState().settled("t1")
    expect(useCanvasTool.getState().postedThreadId).toBeNull()
  })

  it("holds one open turn, dropped by a new draft, a thread, a mode change or close", () => {
    const turn = {
      requestId: "r1",
      chatId: "c1",
      panelId: "p1",
      anchor: draft.anchor,
      at: draft.at,
      surface: "s1",
    }
    const store = useCanvasTool.getState()
    store.setTool("ask")
    store.openDraft({ ...draft, tool: "ask" })
    useCanvasTool.getState().showTurn(turn)
    expect(useCanvasTool.getState()).toMatchObject({
      tool: "ask",
      draft: null,
      openTurn: turn,
    })
    useCanvasTool.getState().openThread("t1")
    expect(useCanvasTool.getState().openTurn).toBeNull()
    useCanvasTool.getState().showTurn(turn)
    expect(useCanvasTool.getState().openThreadId).toBeNull()
    useCanvasTool.getState().openDraft(draft)
    expect(useCanvasTool.getState().openTurn).toBeNull()
    useCanvasTool.getState().showTurn(turn)
    useCanvasTool.getState().close()
    expect(useCanvasTool.getState()).toMatchObject({
      tool: "ask",
      openTurn: null,
    })
    useCanvasTool.getState().showTurn(turn)
    useCanvasTool.getState().setTool("select")
    expect(useCanvasTool.getState().openTurn).toBeNull()
  })

  it("opens a key draft on the whole panel with no spot", () => {
    openKeyDraft("p2", "ask")
    expect(useCanvasTool.getState().draft).toEqual({
      panelId: "p2",
      tool: "ask",
      anchor: null,
      at: null,
      surface: null,
    })
  })
})

async function renderTools(
  permissions: { comment: boolean; edit: boolean },
  agentAvailable: boolean,
  idle = true
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
      <CanvasTools permissions={permissions} idle={idle} />
    </>,
    { queryClient, route: { path: "/", url: "/" } }
  )
  await screen.findByText("page")
}

describe("activeTool", () => {
  const may = { comment: true, ask: true, idle: true }

  it("keeps the chosen mode while it is allowed", () => {
    expect(activeTool("ask", may)).toBe("ask")
    expect(activeTool("comment", may)).toBe("comment")
  })

  it("falls back to Select while the layout is being edited", () => {
    expect(activeTool("ask", { ...may, idle: false })).toBe("select")
  })

  it("falls back to Select for Ask when no model is connected", () => {
    expect(activeTool("ask", { ...may, ask: false })).toBe("select")
    expect(activeTool("comment", { ...may, ask: false })).toBe("comment")
  })
})

describe("CanvasTools", () => {
  it("shows Select and Comment to a Commenter without an agent", async () => {
    await renderTools({ comment: true, edit: false }, false)
    expect(
      screen.getByRole("toolbar", { name: "Canvas tools" })
    ).toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Select" })).toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Comment" })).toBeInTheDocument()
    expect(
      screen.queryByRole("button", { name: "Ask" })
    ).not.toBeInTheDocument()
  })

  it("shows Ask to a Commenter with an agent, since an ask needs only Comment", async () => {
    await renderTools({ comment: true, edit: false }, true)
    expect(screen.getByRole("button", { name: "Ask" })).toBeEnabled()
  })

  it("names the key of each tool", async () => {
    await renderTools({ comment: true, edit: false }, true)
    expect(screen.getByRole("button", { name: "Select" })).toHaveAttribute(
      "aria-keyshortcuts",
      "Escape"
    )
    expect(screen.getByRole("button", { name: "Comment" })).toHaveAttribute(
      "aria-keyshortcuts",
      "C"
    )
    expect(screen.getByRole("button", { name: "Ask" })).toHaveAttribute(
      "aria-keyshortcuts",
      "A"
    )
  })

  it("shows Comment mode until Select is chosen", async () => {
    const user = userEvent.setup()
    await renderTools({ comment: true, edit: false }, false)
    await user.click(screen.getByRole("button", { name: "Comment" }))
    expect(screen.getByRole("toolbar")).toHaveAttribute("data-tool", "comment")
    expect(screen.getByRole("button", { name: "Comment" })).toHaveAttribute(
      "aria-pressed",
      "true"
    )
    expect(screen.getByRole("status")).toHaveTextContent(commentModeText)
    await user.click(screen.getByRole("button", { name: "Select" }))
    expect(screen.getByRole("toolbar")).toHaveAttribute("data-tool", "select")
    expect(screen.queryByRole("status")).not.toBeInTheDocument()
  })

  it("shows Ask mode until Select is chosen", async () => {
    const user = userEvent.setup()
    await renderTools({ comment: true, edit: false }, true)
    await user.click(screen.getByRole("button", { name: "Ask" }))
    expect(useCanvasTool.getState().tool).toBe("ask")
    expect(screen.getByRole("toolbar")).toHaveAttribute("data-tool", "ask")
    expect(screen.getByRole("button", { name: "Ask" })).toHaveAttribute(
      "aria-pressed",
      "true"
    )
    expect(screen.getByRole("status")).toHaveTextContent(askModeText)
    await user.click(screen.getByRole("button", { name: "Select" }))
    expect(screen.getByRole("toolbar")).toHaveAttribute("data-tool", "select")
  })

  it("shows Select while the layout is being edited, with Ask turned off", async () => {
    useCanvasTool.setState({ tool: "ask" })
    await renderTools({ comment: true, edit: true }, true, false)
    expect(screen.getByRole("toolbar")).toHaveAttribute("data-tool", "select")
    expect(screen.getByRole("button", { name: "Ask" })).toBeDisabled()
    expect(screen.queryByRole("status")).not.toBeInTheDocument()
  })

  it.each([
    ["a Viewer", { comment: false, edit: false }, true],
    ["someone who cannot comment", { comment: false, edit: true }, true],
  ])("hides the whole bar for %s", async (_who, permissions, agent) => {
    await renderTools(permissions, agent)
    expect(screen.queryByRole("toolbar")).not.toBeInTheDocument()
    expect(
      screen.queryByRole("button", { name: "Select" })
    ).not.toBeInTheDocument()
  })
})
