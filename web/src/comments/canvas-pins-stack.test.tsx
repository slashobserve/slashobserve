import { screen, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { threadKeys } from "@/api/keys"
import { ungated } from "@/conversation/delivery"
import { useCanvasTool } from "@/dashboards/canvas-tools"
import { createAnchorLayoutStore } from "@/panels/anchor-layout"
import { renderWithProviders, testQueryClient } from "@/test/render"

import { CanvasPins } from "./canvas-pins"
import type { Message, Thread } from "./thread-queries"

vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  useRouteContext: () => ({ principal: { id: "u1", role: "editor" } }),
}))

const actor: Message["actor"] = {
  kind: "user",
  id: "u1",
  name: "Ada",
  origin: "web",
  client: null,
  run_id: null,
  label: "Ada",
}

function thread(id: string, x: number): Thread {
  return {
    id,
    target: { kind: "dashboard", id: "d1" },
    dashboard_id: "d1",
    version: 1,
    panel_id: "p1",
    panel_present: true,
    status: "open",
    allowed_actions: ["reply"],
    unread: false,
    created_at: `2026-11-06T09:00:0${id.slice(1)}Z`,
    updated_at: "2026-11-06T09:00:00Z",
    resolved_at: null,
    resolved_actor: null,
    resolved_by_version: null,
    revision: null,
    anchor: { kind: "position", pos: { x, y: 0.5 } },
    block_id: null,
    agent: { kind: "builtin", id: null },
    agent_off: false,
  } as Thread
}

function message(threadId: string, body: string): Message {
  return {
    id: `${threadId}-m`,
    thread_id: threadId,
    body,
    actor,
    created_at: "2026-11-06T09:00:00Z",
    updated_at: "2026-11-06T09:00:00Z",
    edited_at: null,
    mentions: [],
    can_edit: false,
    can_delete: false,
    block_id: null,
    reply_to: null,
  } as Message
}

function renderPins() {
  const queryClient = testQueryClient()
  queryClient.setQueryData(threadKeys.list("dashboard:d1"), {
    pages: [
      {
        threads: [thread("t1", 0.5), thread("t2", 0.51), thread("t3", 0.9)],
        messages: [
          message("t1", "Spike at noon"),
          message("t2", "Same here"),
          message("t3", "Quiet tail"),
        ],
        next_cursor: null,
        can_comment: true,
      },
    ],
    pageParams: [undefined],
  })
  renderWithProviders(
    <div className="relative">
      <CanvasPins
        dashboardId="d1"
        panelId="p1"
        panelTitle="Latency"
        store={createAnchorLayoutStore()}
        body={{ width: 400, height: 200 }}
        view="data"
        gateInput={ungated}
      />
    </div>,
    { queryClient, route: { path: "/", url: "/" } }
  )
}

beforeEach(() => {
  useCanvasTool.setState({
    tool: "select",
    draft: null,
    openThreadId: null,
    postedThreadId: null,
  })
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response("{}", { status: 200 }))
  )
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("CanvasPins", () => {
  it("stacks overlapping pins into one marker with a count", async () => {
    renderPins()
    expect(
      await screen.findByRole("button", { name: "2 comments here" })
    ).toHaveTextContent("2")
    expect(
      screen.getByRole("button", { name: "Comment by Ada: Quiet tail" })
    ).toBeInTheDocument()
  })

  it("opens a list of the stacked threads and then one of them", async () => {
    const user = userEvent.setup()
    renderPins()
    await user.click(
      await screen.findByRole("button", { name: "2 comments here" })
    )
    const list = await screen.findByRole("dialog", {
      name: "Comments here on Latency",
    })
    await user.click(
      within(list).getByRole("button", { name: "Comment by Ada: Same here" })
    )
    expect(useCanvasTool.getState().openThreadId).toBe("t2")
    const shown = await screen.findByRole("dialog", {
      name: "Thread on Latency",
    })
    expect(shown).toHaveTextContent("Same here")
    await user.click(
      within(shown).getByRole("button", { name: /All 2 comments here/ })
    )
    expect(useCanvasTool.getState().openThreadId).toBeNull()
    expect(
      await screen.findByRole("dialog", { name: "Comments here on Latency" })
    ).toBeInTheDocument()
  })
})
