import { describe, expect, it } from "vitest"

import { sendToAgentState } from "./agent-gate"

const open = {
  canEdit: true,
  agentAvailable: true,
  pinned: false,
  layoutEditing: false,
  activeRequests: 0,
}

describe("sendToAgentState", () => {
  it("allows a request on the latest version with room in the queue", () => {
    expect(sendToAgentState(open)).toEqual({ kind: "allowed" })
    expect(sendToAgentState({ ...open, activeRequests: 4 })).toEqual({
      kind: "allowed",
    })
  })

  it("counts unknown availability as available", () => {
    expect(sendToAgentState({ ...open, agentAvailable: undefined })).toEqual({
      kind: "allowed",
    })
  })

  it("reports no model when the agent is unavailable", () => {
    expect(sendToAgentState({ ...open, agentAvailable: false })).toEqual({
      kind: "no-model",
    })
  })

  it("blocks on a pinned version", () => {
    expect(sendToAgentState({ ...open, pinned: true })).toEqual({
      kind: "blocked",
      reason:
        "You are viewing an older version. Open the latest version to send changes to the agent.",
    })
  })

  it("blocks while the layout is being edited", () => {
    expect(sendToAgentState({ ...open, layoutEditing: true })).toEqual({
      kind: "blocked",
      reason: "Finish editing the layout to send changes to the agent.",
    })
  })

  it("blocks when the queue is full", () => {
    const reason =
      "5 requests are already waiting for the agent. Send this when one finishes."
    expect(sendToAgentState({ ...open, activeRequests: 5 })).toEqual({
      kind: "blocked",
      reason,
    })
    expect(sendToAgentState({ ...open, activeRequests: 9 })).toEqual({
      kind: "blocked",
      reason,
    })
  })

  it("hides the option without edit permission before every other reason", () => {
    expect(
      sendToAgentState({
        canEdit: false,
        agentAvailable: false,
        pinned: true,
        layoutEditing: true,
        activeRequests: 5,
      })
    ).toEqual({ kind: "hidden" })
    expect(sendToAgentState({ ...open, canEdit: false })).toEqual({
      kind: "hidden",
    })
  })

  it("puts no model before every other reason", () => {
    expect(
      sendToAgentState({
        ...open,
        agentAvailable: false,
        pinned: true,
        layoutEditing: true,
        activeRequests: 5,
      })
    ).toEqual({ kind: "no-model" })
  })

  it("puts pinned before layout editing and a full queue", () => {
    expect(
      sendToAgentState({
        ...open,
        pinned: true,
        layoutEditing: true,
        activeRequests: 5,
      })
    ).toMatchObject({
      kind: "blocked",
      reason: expect.stringContaining("older"),
    })
  })

  it("puts layout editing before a full queue", () => {
    expect(
      sendToAgentState({ ...open, layoutEditing: true, activeRequests: 5 })
    ).toMatchObject({
      kind: "blocked",
      reason: expect.stringContaining("layout"),
    })
  })
})
