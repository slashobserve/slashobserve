import { describe, expect, it } from "vitest"

import type { Comment } from "@/comments/queries"
import type { Revision } from "@/revisions/queries"

import type { AgentGate } from "./agent-gate"
import { threadState, threadVerbText } from "./thread-view"

const author = { kind: "user", id: "u1", name: "Ada" } as const
const actor = {
  ...author,
  origin: "web" as const,
  client: null,
  run_id: null,
  label: "Ada",
}
const other = { kind: "user", id: "u2", name: "Grace" } as const
const otherActor = {
  ...other,
  origin: "web" as const,
  client: null,
  run_id: null,
  label: "Grace",
}

function at(seconds: number): string {
  return new Date(Date.UTC(2026, 0, 1, 0, 0, seconds)).toISOString()
}

function root(overrides: Partial<Comment> = {}): Comment {
  return {
    id: "c1",
    dashboard_id: "d1",
    parent_id: null,
    version: 1,
    panel_id: null,
    body: "text",
    status: "open",
    allowed_actions: ["reply", "resolve"],
    author,
    actor,
    created_at: at(0),
    updated_at: at(0),
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

function request(overrides: Partial<Revision> = {}): Revision {
  return {
    id: "r1",
    dashboard_id: "d1",
    allowed_actions: [],
    source: "comment",
    comment_id: "c1",
    panel_id: null,
    prompt: "do it",
    base_version: 1,
    status: "queued",
    state: "queued",
    result_kind: "none",
    attempts: 0,
    result_version: null,
    error: null,
    requested_by: author,
    origin: "web",
    origin_client: null,
    created_at: at(0),
    started_at: null,
    finished_at: null,
    failure_code: null,
    build: null,
    build_saved_at: null,
    continues_id: null,
    continued_by: null,
    continue_note: null,
    ...overrides,
  }
}

const allowed: AgentGate = { kind: "allowed" }
const blocked: AgentGate = { kind: "blocked", reason: "Older version." }
const noModel: AgentGate = { kind: "no-model" }

function state(
  overrides: {
    root?: Partial<Comment>
    request?: Revision | null
    gate?: AgentGate
  } = {}
) {
  return threadState({
    root: root(overrides.root),
    request: overrides.request === undefined ? null : overrides.request,
    gate: overrides.gate ?? allowed,
    clientName: "Claude Code",
  })
}

describe("threadState", () => {
  it("reads Open for an open thread with the built-in agent", () => {
    expect(state({ gate: allowed })).toEqual({ kind: "note", text: "Open" })
    expect(state({ gate: blocked })).toEqual({ kind: "note", text: "Open" })
  })

  it("names the client when there is no built-in agent", () => {
    expect(state({ gate: noModel })).toEqual({
      kind: "note",
      text: "Open, waiting for Claude Code",
    })
  })

  it("reports a sent thread while its request is active", () => {
    expect(state({ request: request({ status: "queued" }) })).toEqual({
      kind: "sent",
    })
    expect(state({ request: request({ status: "running" }) })).toEqual({
      kind: "sent",
    })
  })

  it("reports a failed request on an open thread", () => {
    expect(
      state({ request: request({ status: "failed", error: "boom" }) })
    ).toEqual({ kind: "failed", error: "boom" })
  })

  it("says nothing changed when the request succeeded without a version", () => {
    expect(
      state({ request: request({ status: "succeeded", result_version: null }) })
    ).toEqual({ kind: "note", text: "Finished, nothing changed" })
    expect(
      state({
        request: request({ status: "succeeded", result_version: null }),
        gate: noModel,
      })
    ).toEqual({ kind: "note", text: "Finished, nothing changed" })
  })

  it("keeps the resolved copy over the no-change copy", () => {
    expect(
      state({
        root: { status: "resolved", resolved_by_version: 4 },
        request: request({ status: "succeeded", result_version: null }),
      })
    ).toEqual({ kind: "resolved", text: "Resolved by version 4" })
  })

  it("names the resolver when no version fixed the thread", () => {
    expect(
      state({ root: { status: "resolved", resolved_actor: otherActor } })
    ).toEqual({ kind: "resolved", text: "Resolved by Grace" })
    expect(
      state({
        root: {
          status: "resolved",
          resolved_actor: {
            ...otherActor,
            run_id: "r1",
            label: "/observe for Grace",
          },
        },
      })
    ).toEqual({ kind: "resolved", text: "Resolved by /observe for Grace" })
    expect(state({ root: { status: "resolved" } })).toEqual({
      kind: "resolved",
      text: "Resolved by someone",
    })
  })
})

describe("threadVerbText", () => {
  it("comments when the agent was not asked with the comment", () => {
    expect(threadVerbText(false, null, null)).toBe("commented")
    expect(threadVerbText(false, "p1", "Latency")).toBe("commented on")
    expect(threadVerbText(false, "p1", null)).toBe(
      "commented on a removed panel"
    )
  })

  it("asks the agent when the request came with the comment", () => {
    expect(threadVerbText(true, null, null)).toBe("asked the agent")
    expect(threadVerbText(true, "p1", "Latency")).toBe("asked the agent on")
    expect(threadVerbText(true, "p1", null)).toBe(
      "asked the agent on a removed panel"
    )
  })
})
