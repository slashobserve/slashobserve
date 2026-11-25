import { describe, expect, it } from "vitest"

import type { Message } from "@/comments/thread-queries"

import type { AgentGate } from "./delivery"
import {
  askedAgentName,
  panelText,
  replyCountText,
  runLineText,
  runProgress,
  threadStatus,
  unansweredRun,
  type Run,
} from "./thread-state"

const open: AgentGate = { kind: "allowed" }
const noModel: AgentGate = { kind: "no-model" }

function actor(runId: string | null): Message["actor"] {
  return {
    kind: "user",
    id: "u1",
    name: "Ada",
    origin: "web",
    client: null,
    run_id: runId,
    label: "Ada",
  }
}

const builtinRun: Run = {
  id: "r1",
  status: "running",
  assigned_kind: "builtin",
}

describe("threadStatus", () => {
  it("credits the version that resolved the thread", () => {
    expect(
      threadStatus(
        { status: "resolved", resolved_by_version: 4, resolved_actor: null },
        open,
        "Claude Code"
      )
    ).toEqual({ kind: "resolved", text: "Resolved by version 4" })
  })

  it("names the person who resolved the thread, or someone", () => {
    const resolvedActor = actor(null)
    expect(
      threadStatus(
        {
          status: "resolved",
          resolved_by_version: null,
          resolved_actor: { ...resolvedActor, label: "Grace" },
        },
        open,
        "Claude Code"
      ).text
    ).toBe("Resolved by Grace")
    expect(
      threadStatus(
        { status: "resolved", resolved_by_version: null, resolved_actor: null },
        open,
        "Claude Code"
      ).text
    ).toBe("Resolved by someone")
  })

  it("says who an open thread waits for when no model is set up", () => {
    const thread = {
      status: "open",
      resolved_by_version: null,
      resolved_actor: null,
    } as const
    expect(threadStatus(thread, noModel, "Claude Code")).toEqual({
      kind: "open",
      text: "Open, waiting for Claude Code",
    })
    expect(threadStatus(thread, open, "Claude Code").text).toBe("Open")
  })
})

describe("panelText", () => {
  it("says nothing for a dashboard thread and names a removed panel", () => {
    expect(panelText(null, null)).toBeUndefined()
    expect(panelText("p1", "Errors")).toBe("on")
    expect(panelText("p1", null)).toBe("on a removed panel")
  })
})

describe("unansweredRun", () => {
  it("is the run until a message from that run arrives", () => {
    const question = { run: builtinRun }
    expect(unansweredRun(question, [{ actor: actor(null) }])).toBe(builtinRun)
    expect(
      unansweredRun(question, [{ actor: actor(null) }, { actor: actor("r1") }])
    ).toBeNull()
  })

  it("is null for a message that started no run", () => {
    expect(unansweredRun({ run: null }, [])).toBeNull()
    expect(unansweredRun({}, [])).toBeNull()
  })
})

describe("askedAgentName", () => {
  it("names the built-in agent, the mentioned outside agent, or the agent", () => {
    expect(askedAgentName({ mentions: [] }, builtinRun)).toBe("Observe Agent")
    const mentions: Message["mentions"] = [
      { kind: "grant", id: "g1", name: "Claude Code" },
    ]
    expect(askedAgentName({ mentions }, { assigned_kind: "grant" })).toBe(
      "Claude Code"
    )
    expect(askedAgentName({ mentions: [] }, { assigned_kind: "grant" })).toBe(
      "The agent"
    )
  })
})

describe("runLineText", () => {
  it.each([
    ["queued", "Observe Agent is working on it"],
    ["running", "Observe Agent is working on it"],
    ["succeeded", "Observe Agent finished"],
    ["failed", "Observe Agent could not finish"],
    ["canceling", "Stopping"],
    ["canceled", "Stopped"],
  ] as const)("words a built-in run that is %s", (status, text) => {
    expect(runLineText(builtinRun, status, "Observe Agent")).toBe(text)
  })

  it("only says an outside agent was asked, whatever its status", () => {
    expect(
      runLineText({ assigned_kind: "grant" }, "running", "Claude Code")
    ).toBe("Claude Code was asked")
  })
})

describe("runProgress", () => {
  it.each([
    [{ status: "running", state: "canceling" }, "canceling"],
    [{ status: "failed", state: "canceled" }, "canceled"],
    [{ status: "running", state: "running" }, "running"],
    [{ status: "succeeded" }, "succeeded"],
    [undefined, "queued"],
  ] as const)("reads %j as %s", (request, expected) => {
    expect(runProgress(request, "queued")).toBe(expected)
  })
})

describe("replyCountText", () => {
  it("counts replies", () => {
    expect(replyCountText(1)).toBe("1 reply")
    expect(replyCountText(3)).toBe("3 replies")
  })
})
