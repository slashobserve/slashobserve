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
  status: Comment["status"]
): Extract<TimelineEntry, { kind: "thread" }> {
  const resolved = status === "resolved"
  const root = comment({
    status,
    resolved_at: resolved ? "2026-11-06T11:00:00Z" : null,
    resolved_by: resolved ? grace : null,
    resolved_actor: resolved
      ? { ...graceViaMcp, origin: "web", client: null, label: "Grace" }
      : null,
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

function threadView(status: Comment["status"], client: QueryClient) {
  return (
    <QueryClientProvider client={client}>
      <ol>
        <ThreadEntry
          dashboardId="dashboard"
          entry={thread(status)}
          panelTitle={null}
          onSelectPanel={() => {}}
          gate={{ kind: "hidden" }}
          canComment
          latestVersion={1}
          clientName="Claude Code"
        />
      </ol>
    </QueryClientProvider>
  )
}

function renderThread(status: Comment["status"]) {
  const client = new QueryClient()
  const { rerender } = render(threadView(status, client))
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
})
