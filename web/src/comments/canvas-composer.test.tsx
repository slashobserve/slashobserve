import { screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { createRef } from "react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { ungated } from "@/conversation/delivery"
import { useCanvasTool, type CanvasDraft } from "@/dashboards/canvas-tools"
import type { Anchor } from "@/panels/anchors"
import { renderWithProviders } from "@/test/render"

import {
  boxMinHeight,
  boxMinWidth,
  boxSizeKey,
  parseStoredBoxSizes,
} from "./canvas-box-size"
import {
  CanvasComposer,
  panelTitleOf,
  spotAnchor,
  useCanvasGate,
} from "./canvas-composer"
import type { CanvasSubject } from "./canvas-host"
import { must } from "@/test/must"

vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  useRouteContext: () => ({ principal: { id: "u1", role: "editor" } }),
}))

const onBar: Anchor = {
  kind: "bar",
  pos: { x: 0.3, y: 0.6 },
  category: "checkout",
  series: { name: "p99" },
  shown: "840 ms",
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  })
}

function stubServer() {
  const posts: { path: string; body: Record<string, unknown> }[] = []
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const { pathname } = new URL(request.url)
      if (request.method === "POST") {
        posts.push({ path: pathname, body: await request.clone().json() })
        return json(
          {
            thread: { id: "t9" },
            message: { id: "m1", thread_id: "t9", body: "" },
            to: "everyone",
            replayed: false,
          },
          201
        )
      }
      return json({ service_accounts: [], connected: [], available: true })
    })
  )
  return posts
}

function renderDraft(
  draft: CanvasDraft,
  subject: CanvasSubject = { kind: "dashboard", id: "d1" }
) {
  useCanvasTool.setState({ tool: "comment", draft })
  const surfaceRef = createRef<HTMLElement>()
  return renderWithProviders(
    <CanvasComposer
      subject={subject}
      panelTitle="Latency"
      draft={draft}
      surfaceRef={surfaceRef}
      gate={ungated}
    />,
    { route: { path: "/", url: "/" } }
  )
}

const clicked: CanvasDraft = {
  ownerId: "p1",
  tool: "comment",
  anchor: onBar,
  at: { x: 120, y: 80 },
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

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("resizing the box", () => {
  afterEach(() => {
    localStorage.removeItem(boxSizeKey)
  })

  it("resizes from the keyboard, remembers the size and resets on a double click", async () => {
    stubServer()
    const user = userEvent.setup()
    renderDraft(clicked)
    const popup = await screen.findByRole("dialog", {
      name: "Comment on the canvas",
    })
    expect(popup).not.toHaveAttribute("data-sized")
    const grip = within(popup).getByRole("button", { name: "Resize this box" })
    grip.focus()
    await user.keyboard("{ArrowRight}")
    expect(popup).toHaveAttribute("data-sized")
    expect(parseStoredBoxSizes(localStorage.getItem(boxSizeKey))).toEqual({
      comment: { width: boxMinWidth, height: boxMinHeight },
    })
    await user.dblClick(grip)
    expect(popup).not.toHaveAttribute("data-sized")
    expect(localStorage.getItem(boxSizeKey)).toBeNull()
  })

  it("opens at the size kept from last time", async () => {
    localStorage.setItem(
      boxSizeKey,
      JSON.stringify({ comment: { width: 480, height: 400 } })
    )
    stubServer()
    renderDraft(clicked)
    const popup = await screen.findByRole("dialog", {
      name: "Comment on the canvas",
    })
    expect(popup).toHaveAttribute("data-sized")
  })
})

function GateProbe({ layoutEditing }: { layoutEditing: boolean }) {
  const gate = useCanvasGate(true, layoutEditing)
  return <output>{gate.layoutEditing ? "editing" : "idle"}</output>
}

describe("useCanvasGate", () => {
  it("passes on whether the layout has unsaved moves", async () => {
    stubServer()
    renderWithProviders(<GateProbe layoutEditing />, {
      route: { path: "/", url: "/" },
    })
    expect(await screen.findByRole("status")).toHaveTextContent("editing")
  })
})

describe("CanvasComposer", () => {
  it("opens at the click with the target chips and the field focused", async () => {
    stubServer()
    renderDraft(clicked)
    const popup = await screen.findByRole("dialog", {
      name: "Comment on the canvas",
    })
    expect(popup).toHaveAttribute("data-opened-by", "pointer")
    const chips = within(popup).getByRole("list", { name: "Context" })
    expect(chips).toHaveTextContent("Latency")
    expect(chips).toHaveTextContent("Bar checkout of the series name=p99")
    await waitFor(() =>
      expect(
        within(popup).getByRole("textbox", { name: "Write a comment" })
      ).toHaveFocus()
    )
  })

  it("posts a thread with the anchor and marks it as just posted", async () => {
    const posts = stubServer()
    const user = userEvent.setup()
    renderDraft(clicked)
    const field = await screen.findByRole("textbox", {
      name: "Write a comment",
    })
    await user.type(field, "Why the spike?{Enter}")
    await waitFor(() => expect(posts).toHaveLength(1))
    expect(must(posts[0]).path).toBe("/api/v1/threads")
    expect(must(posts[0]).body).toMatchObject({
      target: "dashboard:d1",
      body: "Why the spike?",
      panel_id: "p1",
      anchor: onBar,
    })
    await waitFor(() =>
      expect(useCanvasTool.getState()).toMatchObject({
        tool: "comment",
        draft: null,
        postedThreadId: "t9",
      })
    )
  })

  it("asks the agent when the comment mentions it, and the chip follows", async () => {
    const posts = stubServer()
    const user = userEvent.setup()
    renderDraft(clicked)
    const popup = await screen.findByRole("dialog", {
      name: "Comment on the canvas",
    })
    const field = within(popup).getByRole("textbox", {
      name: "Write a comment",
    })
    expect(within(popup).getByText("To: everyone here")).toBeInTheDocument()
    await user.type(field, "@observe make this a bar chart")
    expect(within(popup).getByText("To: Observe Agent")).toBeInTheDocument()
    await user.keyboard("{Enter}")
    await waitFor(() => expect(posts).toHaveLength(1))
    expect(must(posts[0]).body).toMatchObject({
      body: "@observe make this a bar chart",
      to: "agent",
    })
  })

  it("sends a comment without a mention to everyone", async () => {
    const posts = stubServer()
    const user = userEvent.setup()
    renderDraft(clicked)
    const field = await screen.findByRole("textbox", {
      name: "Write a comment",
    })
    await user.type(field, "Why the spike?{Enter}")
    await waitFor(() => expect(posts).toHaveLength(1))
    expect(must(posts[0]).body).toMatchObject({ to: "everyone" })
  })

  it("does not send a lone @ when Enter is pressed with the mention list open", async () => {
    const posts = stubServer()
    const user = userEvent.setup()
    renderDraft(clicked)
    const field = await screen.findByRole("textbox", {
      name: "Write a comment",
    })
    await user.type(field, "@")
    await screen.findAllByRole("option")
    await user.keyboard("{Enter}")
    expect(field).toHaveValue("@")
    expect(posts).toHaveLength(0)
    await user.keyboard("{ArrowDown}{Enter}")
    await waitFor(() => expect(field).toHaveValue("@observe "))
    expect(posts).toHaveLength(0)
  })

  it("posts a thread on an observation block with the selected text", async () => {
    const posts = stubServer()
    const user = userEvent.setup()
    const quoted = {
      kind: "text",
      pos: { x: 0.25, y: 0.5 },
      quote: "checkout errors rose",
    } as const
    renderDraft(
      { ...clicked, ownerId: "b1", anchor: quoted },
      {
        kind: "observation",
        id: "o1",
      }
    )
    const field = await screen.findByRole("textbox", {
      name: "Write a comment",
    })
    await user.type(field, "Which deploy?{Enter}")
    await waitFor(() => expect(posts).toHaveLength(1))
    expect(must(posts[0]).body).toMatchObject({
      target: "observation:o1",
      block_id: "b1",
      anchor: quoted,
    })
    expect(must(posts[0]).body).not.toHaveProperty("panel_id")
  })

  it("opens by key on the whole panel with no motion", async () => {
    stubServer()
    renderDraft({ ...clicked, anchor: null, at: null, surface: null })
    const popup = await screen.findByRole("dialog", {
      name: "Comment on the canvas",
    })
    expect(popup).toHaveAttribute("data-opened-by", "key")
    expect(popup.className).not.toMatch(/data-starting-style/)
    expect(
      within(popup).getByRole("list", { name: "Context" })
    ).toHaveTextContent("Latency, whole panel")
  })

  it("stays open with its text when you click outside after typing", async () => {
    stubServer()
    const user = userEvent.setup()
    renderDraft(clicked)
    const field = await screen.findByRole("textbox", {
      name: "Write a comment",
    })
    await user.type(field, "half a thought")
    await user.click(document.body)
    expect(
      screen.getByRole("dialog", { name: "Comment on the canvas" })
    ).toBeInTheDocument()
    expect(field).toHaveValue("half a thought")
    expect(useCanvasTool.getState().draft).toBe(clicked)
  })

  it("leaves the mode on the first Escape while the comment is still being sent", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (request: Request) =>
        request.method === "POST"
          ? new Promise<Response>(() => {})
          : json({ service_accounts: [], connected: [] })
      )
    )
    const user = userEvent.setup()
    renderDraft(clicked)
    const field = await screen.findByRole("textbox", {
      name: "Write a comment",
    })
    await user.type(field, "Why the spike?{Enter}")
    await waitFor(() => expect(useCanvasTool.getState().sending).toBe(true))
    await user.keyboard("{Escape}")
    await waitFor(() =>
      expect(useCanvasTool.getState()).toMatchObject({
        tool: "select",
        draft: null,
        leaving: null,
      })
    )
  })

  it("closes on a click outside while it is empty", async () => {
    stubServer()
    const user = userEvent.setup()
    renderDraft(clicked)
    await screen.findByRole("textbox", { name: "Write a comment" })
    await user.click(document.body)
    await waitFor(() => expect(useCanvasTool.getState().draft).toBeNull())
  })

  it("keeps its text when it is mounted again for the same draft", async () => {
    stubServer()
    const user = userEvent.setup()
    const view = renderDraft(clicked)
    await user.type(
      await screen.findByRole("textbox", { name: "Write a comment" }),
      "half a thought"
    )
    view.unmount()
    renderDraft(clicked)
    expect(
      await screen.findByRole("textbox", { name: "Write a comment" })
    ).toHaveValue("half a thought")
  })
})

function elementAt(left: number, top: number): HTMLElement {
  const element = document.createElement("div")
  element.getBoundingClientRect = () => DOMRect.fromRect({ x: left, y: top })
  return element
}

describe("spotAnchor", () => {
  it("measures the element the ref holds when the box is placed", () => {
    const spot = { current: elementAt(300, 140) as HTMLElement | null }
    const anchor = spotAnchor(spot, null)
    expect(anchor.getBoundingClientRect()).toMatchObject({ x: 312, y: 152 })
    spot.current = elementAt(310, 180)
    expect(anchor.getBoundingClientRect()).toMatchObject({ x: 322, y: 192 })
  })

  it("offsets a clicked point from the element", () => {
    const anchor = spotAnchor(
      { current: elementAt(300, 140) },
      { x: 20, y: 30 }
    )
    expect(anchor.getBoundingClientRect()).toMatchObject({ x: 320, y: 170 })
  })
})

describe("panelTitleOf", () => {
  it("finds the title from the surface and from the wrapper around the panel", () => {
    const wrapper = document.createElement("div")
    const article = document.createElement("article")
    const title = document.createElement("button")
    title.setAttribute("data-panel-title", "")
    const surface = document.createElement("div")
    article.append(title, surface)
    wrapper.append(article)
    expect(panelTitleOf(wrapper)).toBe(title)
    expect(panelTitleOf(surface)).toBe(title)
    expect(panelTitleOf(null)).toBeNull()
  })
})
