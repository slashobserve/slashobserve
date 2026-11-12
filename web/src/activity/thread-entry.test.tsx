import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it } from "vitest"

import type { Comment } from "@/comments/queries"

import { ThreadEntry } from "./thread-entry"
import type { TimelineEntry } from "./timeline-model"

const ada = { kind: "user" as const, id: "user-ada", name: "Ada" }
const actor = {
  ...ada,
  origin: "web" as const,
  client: null,
  run_id: null,
  label: "Ada",
}
const grace = { kind: "user" as const, id: "user-grace", name: "Grace" }
const graceViaMcp = {
  ...grace,
  origin: "mcp" as const,
  client: "Claude Code",
  run_id: null,
  label: "Grace (via Claude Code)",
}

function comment(overrides: Partial<Comment>): Comment {
  return {
    id: "root",
    dashboard_id: "dashboard",
    parent_id: null,
    version: 1,
    panel_id: null,
    body: "Latency looks high",
    status: "open",
    allowed_actions: ["reply", "resolve"],
    author: ada,
    actor,
    created_at: "2026-11-06T10:00:00Z",
    updated_at: "2026-11-06T10:00:00Z",
    resolved_at: null,
    resolved_by: null,
    resolved_actor: null,
    edited_at: null,
    mentions: [],
    can_edit: false,
    resolved_by_version: null,
    panel_present: true,
    revision: null,
    ...overrides,
  }
}

function thread(
  status: Comment["status"],
  edits: { root?: Partial<Comment>; reply?: Partial<Comment> } = {}
): Extract<TimelineEntry, { kind: "thread" }> {
  const resolved = status === "resolved"
  const root = comment({
    status,
    allowed_actions: resolved ? ["reply", "reopen"] : ["reply", "resolve"],
    resolved_at: resolved ? "2026-11-06T11:00:00Z" : null,
    resolved_by: resolved ? grace : null,
    resolved_actor: resolved
      ? { ...graceViaMcp, origin: "web", client: null, label: "Grace" }
      : null,
    ...edits.root,
  })
  const reply = comment({
    id: "reply",
    parent_id: "root",
    body: "Looking now",
    status,
    author: grace,
    actor: graceViaMcp,
    created_at: "2026-11-06T10:30:00Z",
    updated_at: "2026-11-06T10:30:00Z",
    ...edits.reply,
  })
  return {
    kind: "thread",
    key: "thread:root",
    at: root.created_at,
    panelId: null,
    thread: { root, replies: [reply] },
    request: null,
    askedAgent: false,
  }
}

function threadView(
  status: Comment["status"],
  client: QueryClient,
  edits?: Parameters<typeof thread>[1]
) {
  return (
    <QueryClientProvider client={client}>
      <ol>
        <ThreadEntry
          dashboardId="dashboard"
          entry={thread(status, edits)}
          panelTitle={null}
          onSelectPanel={() => {}}
          gate={{ kind: "hidden" }}
          latestVersion={1}
          clientName="Claude Code"
        />
      </ol>
    </QueryClientProvider>
  )
}

function renderThread(
  status: Comment["status"],
  edits?: Parameters<typeof thread>[1]
) {
  const client = new QueryClient()
  const { rerender } = render(threadView(status, client, edits))
  return {
    rerender: (next: Comment["status"]) => rerender(threadView(next, client)),
  }
}

describe("ThreadEntry", () => {
  it("shows an open thread with its replies and actions", () => {
    renderThread("open")
    expect(screen.getByText("Open", { exact: true })).toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Reply" })).toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Resolve" })).toBeInTheDocument()
    expect(screen.getByText("Looking now")).toBeInTheDocument()
  })

  it("shows Reply and no Resolve when the server offers only reply", () => {
    renderThread("open", { root: { allowed_actions: ["reply"] } })
    expect(screen.getByRole("button", { name: "Reply" })).toBeInTheDocument()
    expect(screen.queryByRole("button", { name: "Resolve" })).toBeNull()
    expect(screen.queryByRole("button", { name: "Reopen" })).toBeNull()
  })

  it("shows nothing to do when the server offers no action", () => {
    renderThread("open", { root: { allowed_actions: [] } })
    expect(screen.queryByRole("button", { name: "Reply" })).toBeNull()
    expect(screen.queryByRole("button", { name: "Resolve" })).toBeNull()
  })

  it("shows Reopen on a resolved thread that the server lets the reader reopen", () => {
    renderThread("resolved", { root: { allowed_actions: ["reply", "reopen"] } })
    expect(screen.getByRole("button", { name: "Reopen" })).toBeInTheDocument()
    expect(screen.queryByRole("button", { name: "Resolve" })).toBeNull()
  })

  it("labels each comment with its actor", () => {
    renderThread("open")
    expect(screen.getByText("Ada")).toBeInTheDocument()
    expect(screen.getByText("Grace (via Claude Code)")).toBeInTheDocument()
  })

  it("folds a resolved thread to its first line", () => {
    renderThread("resolved")
    expect(screen.getByText("Resolved by Grace")).toBeInTheDocument()
    expect(screen.getByText("Latency looks high")).toBeInTheDocument()
    expect(screen.getByText("1 reply")).toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Show thread" })).toHaveAttribute(
      "aria-expanded",
      "false"
    )
    expect(screen.queryByText("Looking now")).toBeNull()
  })

  it("shows and hides a resolved thread", async () => {
    const user = userEvent.setup()
    renderThread("resolved")
    await user.click(screen.getByRole("button", { name: "Show thread" }))
    expect(screen.getByText("Looking now")).toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Hide thread" })).toHaveAttribute(
      "aria-expanded",
      "true"
    )
  })

  it("expands a folded thread to reply", async () => {
    const user = userEvent.setup()
    renderThread("resolved")
    await user.click(screen.getByRole("button", { name: "Reply" }))
    expect(screen.getByText("Looking now")).toBeInTheDocument()
    expect(screen.getByRole("textbox", { name: "Reply" })).toHaveFocus()
  })

  it("folds a thread resolved after replying while it was open", async () => {
    const user = userEvent.setup()
    const { rerender } = renderThread("open")
    await user.click(screen.getByRole("button", { name: "Reply" }))
    rerender("resolved")
    expect(screen.getByRole("button", { name: "Show thread" })).toHaveAttribute(
      "aria-expanded",
      "false"
    )
    expect(screen.queryByText("Looking now")).toBeNull()
  })

  it("offers Edit only on the comments the reader may edit", () => {
    renderThread("open", { reply: { can_edit: true } })
    const edits = screen.getAllByRole("button", { name: "Edit" })
    expect(edits).toHaveLength(1)
    expect(edits[0].closest("li")).toHaveTextContent("Looking now")
  })

  it("opens the edit form with the body and closes it on Cancel", async () => {
    const user = userEvent.setup()
    renderThread("open", { root: { can_edit: true } })
    await user.click(screen.getByRole("button", { name: "Edit" }))
    expect(screen.getByRole("textbox", { name: "Edit comment" })).toHaveValue(
      "Latency looks high"
    )
    await user.click(screen.getByRole("button", { name: "Cancel" }))
    expect(screen.queryByRole("textbox", { name: "Edit comment" })).toBeNull()
    expect(screen.getByRole("button", { name: "Edit" })).toHaveFocus()
  })

  it("marks an edited comment", () => {
    renderThread("open", {
      reply: { edited_at: "2026-11-06T10:40:00Z" },
    })
    expect(screen.getAllByText(/edited/)).toHaveLength(1)
  })

  it("shows recorded mentions as names and leaves unrecorded tokens as typed", () => {
    const adaId = "7f1f0d1e-9a3b-4c2d-8e5f-6a7b8c9d0e1f"
    renderThread("open", {
      root: {
        body: `@[Ada](user:${adaId}) and @observe and @[Ghost](user:00000000-0000-0000-0000-000000000000)`,
        mentions: [
          { kind: "user", id: adaId, name: "Ada Lovelace" },
          { kind: "agent", id: "observe", name: "observe" },
        ],
      },
    })
    expect(screen.getByText("@Ada Lovelace")).toBeInTheDocument()
    expect(screen.getByText("@observe")).toBeInTheDocument()
    expect(
      screen.getByText(
        /@\[Ghost\]\(user:00000000-0000-0000-0000-000000000000\)/
      )
    ).toBeInTheDocument()
  })

  it("offers the thread's people and the agent in the reply form", async () => {
    const user = userEvent.setup()
    renderThread("open")
    await user.click(screen.getByRole("button", { name: "Reply" }))
    await user.click(
      screen.getByRole("combobox", { name: "Mention a person or an agent" })
    )
    const options = await screen.findAllByRole("option")
    expect(options.map((option) => option.textContent)).toEqual([
      "Observe Agent@observe",
      "Ada",
      "Grace",
    ])
  })
})
