import { screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { createRef } from "react"
import { beforeEach, describe, expect, it } from "vitest"

import { observeKeys } from "@/api/keys"
import { renderWithProviders, testQueryClient } from "@/test/render"

import {
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
  agentAvailable: boolean
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
      <CanvasTools permissions={permissions} idle composerRef={createRef()} />
    </>,
    { queryClient, route: { path: "/", url: "/" } }
  )
  await screen.findByText("page")
}

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
