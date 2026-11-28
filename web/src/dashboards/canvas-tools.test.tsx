import { act, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

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
  ownerId: "p1",
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
    leaving: null,
    detached: null,
    sending: false,
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
      ownerId: "p1",
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
      ownerId: "p2",
      tool: "ask",
      anchor: null,
      at: null,
      surface: null,
    })
  })
})

function placeBoxes(initial: {
  left?: number
  width?: number
  areaTop?: number
  areaBottom: number
  barHeight?: number
}) {
  const boxes = { left: 0, width: 0, areaTop: 0, barHeight: 40, ...initial }
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(
    function (this: HTMLElement) {
      if (this.dataset.testid === "area") {
        return { top: boxes.areaTop, bottom: boxes.areaBottom } as DOMRect
      }
      return {
        left: boxes.left,
        width: boxes.width,
        height: boxes.barHeight,
      } as DOMRect
    }
  )
  return boxes
}

afterEach(() => vi.restoreAllMocks())

function scrollArea() {
  act(() => {
    screen.getByTestId("area").dispatchEvent(new Event("scroll"))
  })
}

function frames() {
  vi.spyOn(window, "requestAnimationFrame").mockImplementation((run) => {
    run(0)
    return 0
  })
}

async function renderTools(
  permissions: { comment: boolean; edit: boolean },
  agentAvailable: boolean,
  idle = true,
  pageFooter = false
) {
  const queryClient = testQueryClient()
  queryClient.setQueryData(observeKeys.agent(), {
    available: agentAvailable,
    provider: agentAvailable ? "anthropic" : null,
    model: agentAvailable ? "claude-haiku-5-5" : null,
    effort: null,
  })
  renderWithProviders(
    <div data-testid="area" style={{ overflowY: "auto" }}>
      <p>page</p>
      <CanvasTools
        permissions={permissions}
        idle={idle}
        pageFooter={pageFooter}
      />
    </div>,
    { queryClient, route: { path: "/", url: "/" } }
  )
  await screen.findByText("page")
}

describe("leaving with typed text", () => {
  it("asks before a tool change or a close drops the text, and moves on once discarded", () => {
    useCanvasTool.setState({ tool: "comment" })
    useCanvasTool.getState().openDraft(draft)
    useCanvasTool.getState().setText("Which deploy?")
    useCanvasTool.getState().leave("select")
    expect(useCanvasTool.getState()).toMatchObject({
      tool: "comment",
      leaving: "select",
      text: "Which deploy?",
    })
    useCanvasTool.getState().keepTyping()
    expect(useCanvasTool.getState().leaving).toBeNull()
    useCanvasTool.getState().leave("close")
    useCanvasTool.getState().discard()
    expect(useCanvasTool.getState()).toMatchObject({
      tool: "comment",
      draft: null,
      text: "",
      leaving: null,
    })
  })

  it("leaves at once when nothing is typed", () => {
    useCanvasTool.getState().openDraft(draft)
    useCanvasTool.getState().leave("ask")
    expect(useCanvasTool.getState()).toMatchObject({
      tool: "ask",
      draft: null,
      leaving: null,
    })
  })

  it("leaves at once while the text is being sent, and asks again if the send ends unposted", () => {
    useCanvasTool.setState({ tool: "comment" })
    useCanvasTool.getState().openDraft(draft)
    useCanvasTool.getState().setText("Which deploy?")
    useCanvasTool.getState().setSending(true)
    useCanvasTool.getState().leave("select")
    expect(useCanvasTool.getState()).toMatchObject({
      tool: "select",
      draft: null,
      leaving: null,
    })
    useCanvasTool.setState({ tool: "comment" })
    useCanvasTool.getState().openDraft(draft)
    useCanvasTool.getState().setText("Which deploy?")
    useCanvasTool.getState().setSending(false)
    useCanvasTool.getState().leave("select")
    expect(useCanvasTool.getState().leaving).toBe("select")
  })

  it("sets a typed draft apart so the canvas is free again", () => {
    useCanvasTool.getState().openDraft(draft)
    useCanvasTool.getState().setText("Which deploy?")
    useCanvasTool.getState().detach()
    expect(useCanvasTool.getState()).toMatchObject({
      draft: null,
      text: "",
      detached: { tool: "comment", text: "Which deploy?" },
    })
    const other = { ...draft, ownerId: "p2" }
    useCanvasTool.getState().openDraft(other)
    expect(useCanvasTool.getState().draft).toBe(other)
    useCanvasTool.getState().dropDetached()
    expect(useCanvasTool.getState().detached).toBeNull()
  })
})

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

  it("reserves room in the page flow and fixes the toolbar to the viewport bottom only when asked", async () => {
    placeBoxes({ areaBottom: window.innerHeight })
    await renderTools({ comment: true, edit: false }, false, true, true)
    const bar = screen.getByRole("toolbar")
    const slot = bar.parentElement?.parentElement
    expect(slot).toHaveAttribute("data-page-footer")
    expect(slot).toHaveClass("h-24", "-mt-8")
    expect(bar.parentElement).toHaveClass("fixed")
    expect(bar.parentElement).toHaveStyle({ bottom: "16px" })
    expect(bar.parentElement).not.toHaveClass("sticky")
  })

  it("keeps a sticky zero-height box by default so a scrolling sibling owns the room", async () => {
    await renderTools({ comment: true, edit: false }, false)
    const box = screen.getByRole("toolbar").parentElement?.parentElement
    expect(box).toHaveClass("sticky", "h-0")
    expect(box).not.toHaveAttribute("data-page-footer")
    expect(box).not.toHaveClass("h-24")
    expect(box).not.toHaveClass("-mt-8")
    expect(box).not.toHaveClass("fixed")
  })

  it("docks the hint as a strip fixed to the viewport bottom in a page footer", async () => {
    placeBoxes({ areaBottom: window.innerHeight })
    const user = userEvent.setup()
    await renderTools({ comment: true, edit: false }, false, true, true)
    await user.click(screen.getByRole("button", { name: "Comment" }))
    const strip = screen.getByRole("status").parentElement
    expect(strip).toHaveClass("fixed", "border-t")
    expect(strip).toHaveStyle({ bottom: "0px" })
    expect(strip).toContainElement(screen.getByRole("toolbar"))
  })

  it("lays the page footer over the box its slot takes in the column", async () => {
    placeBoxes({ left: 240, width: 612, areaBottom: window.innerHeight })
    await renderTools({ comment: true, edit: false }, false, true, true)
    const fixed = screen.getByRole("toolbar").parentElement
    expect(fixed).toHaveStyle({ left: "240px", width: "612px" })
  })

  it("rides at the bottom of a short column instead of the viewport bottom", async () => {
    placeBoxes({ areaBottom: window.innerHeight - 400 })
    await renderTools({ comment: true, edit: false }, false, true, true)
    const fixed = screen.getByRole("toolbar").parentElement
    expect(fixed).toHaveStyle({ bottom: "416px" })
    expect(fixed).not.toHaveClass("invisible")
  })

  it("sits at its own inset while the column reaches the viewport bottom", async () => {
    placeBoxes({ areaBottom: window.innerHeight })
    await renderTools({ comment: true, edit: false }, false, true, true)
    const fixed = screen.getByRole("toolbar").parentElement
    expect(fixed).toHaveStyle({ bottom: "16px" })
    expect(fixed).not.toHaveClass("invisible")
  })

  it("follows the column bottom as the page scrolls and hides once the column is out of view", async () => {
    frames()
    const boxes = placeBoxes({ areaBottom: window.innerHeight })
    await renderTools({ comment: true, edit: false }, false, true, true)
    const fixed = screen.getByRole("toolbar").parentElement
    boxes.areaBottom = window.innerHeight - 300
    scrollArea()
    expect(fixed).toHaveStyle({ bottom: "316px" })
    expect(fixed).not.toHaveClass("invisible")
    boxes.areaBottom = 0
    scrollArea()
    expect(fixed).toHaveClass("invisible")
    boxes.areaBottom = window.innerHeight
    scrollArea()
    expect(fixed).not.toHaveClass("invisible")
    expect(fixed).toHaveStyle({ bottom: "16px" })
  })

  it("ignores scrolls that are not the column's scroll area or an ancestor of it", async () => {
    frames()
    const boxes = placeBoxes({ areaBottom: window.innerHeight })
    await renderTools({ comment: true, edit: false }, false, true, true)
    const fixed = screen.getByRole("toolbar").parentElement
    boxes.areaBottom = window.innerHeight - 300
    act(() => {
      screen.getByText("page").dispatchEvent(new Event("scroll"))
    })
    expect(fixed).toHaveStyle({ bottom: "16px" })
    act(() => {
      document.dispatchEvent(new Event("scroll"))
    })
    expect(fixed).toHaveStyle({ bottom: "316px" })
  })

  it("batches scroll measurements into one animation frame", async () => {
    const queued: FrameRequestCallback[] = []
    vi.spyOn(window, "requestAnimationFrame").mockImplementation((run) => {
      queued.push(run)
      return queued.length
    })
    const boxes = placeBoxes({ areaBottom: window.innerHeight })
    await renderTools({ comment: true, edit: false }, false, true, true)
    boxes.areaBottom = window.innerHeight - 300
    scrollArea()
    scrollArea()
    scrollArea()
    expect(queued).toHaveLength(1)
    act(() => queued[0]!(0))
    expect(screen.getByRole("toolbar").parentElement).toHaveStyle({
      bottom: "316px",
    })
  })

  it("hides the page footer as soon as the toolbar no longer fits on screen", async () => {
    frames()
    const boxes = placeBoxes({ areaBottom: 56 })
    await renderTools({ comment: true, edit: false }, false, true, true)
    const fixed = screen.getByRole("toolbar").parentElement
    expect(fixed).not.toHaveClass("invisible")
    boxes.areaBottom = 55
    scrollArea()
    expect(fixed).toHaveClass("invisible")
  })

  it("returns to Select when the column scrolls out of view in a drawing mode", async () => {
    frames()
    const user = userEvent.setup()
    const boxes = placeBoxes({ areaBottom: window.innerHeight })
    await renderTools({ comment: true, edit: false }, false, true, true)
    await user.click(screen.getByRole("button", { name: "Comment" }))
    expect(useCanvasTool.getState().tool).toBe("comment")
    boxes.areaBottom = 0
    scrollArea()
    expect(useCanvasTool.getState().tool).toBe("select")
  })

  it("hides the page footer when the column starts below the viewport", async () => {
    placeBoxes({
      areaTop: window.innerHeight + 10,
      areaBottom: window.innerHeight + 900,
    })
    await renderTools({ comment: true, edit: false }, false, true, true)
    expect(screen.getByRole("toolbar").parentElement).toHaveClass("invisible")
  })

  it("docks the strip to the column bottom with no inset", async () => {
    const user = userEvent.setup()
    placeBoxes({ areaBottom: window.innerHeight - 100 })
    await renderTools({ comment: true, edit: false }, false, true, true)
    await user.click(screen.getByRole("button", { name: "Comment" }))
    expect(screen.getByRole("toolbar").parentElement).toHaveStyle({
      bottom: "100px",
    })
  })

  it("lets clicks through the floating strip and keeps the toolbar clickable", async () => {
    await renderTools({ comment: true, edit: false }, false, true, true)
    const bar = screen.getByRole("toolbar")
    expect(bar.parentElement).toHaveClass("pointer-events-none")
    expect(bar).toHaveClass("pointer-events-auto")
  })

  it("keeps the tool button and its focus when the hint docks beside the toolbar", async () => {
    const user = userEvent.setup()
    await renderTools({ comment: true, edit: false }, false)
    const button = screen.getByRole("button", { name: "Comment" })
    await user.click(button)
    expect(screen.getByRole("button", { name: "Comment" })).toBe(button)
    expect(button).toHaveFocus()
    expect(screen.getByRole("status").parentElement).toBe(
      screen.getByRole("toolbar").parentElement?.parentElement
    )
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

  it("asks before Select drops a typed draft", async () => {
    const user = userEvent.setup()
    useCanvasTool.setState({ tool: "comment" })
    useCanvasTool.getState().openDraft(draft)
    useCanvasTool.getState().setText("Which deploy?")
    await renderTools({ comment: true, edit: false }, false)
    await user.click(screen.getByRole("button", { name: "Select" }))
    expect(
      await screen.findByRole("alertdialog", { name: "Discard your draft?" })
    ).toBeInTheDocument()
    await user.click(screen.getByRole("button", { name: "Keep editing" }))
    expect(useCanvasTool.getState()).toMatchObject({
      tool: "comment",
      text: "Which deploy?",
    })
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
