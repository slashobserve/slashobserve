import { describe, expect, test } from "vitest"

import type { RequestSummary } from "@/observe/queries"

import { chatOutcome, chatState, deleteChatText, statesFor } from "./chat-state"

function summary(overrides: Partial<RequestSummary> = {}): RequestSummary {
  return {
    id: "00000000-0000-4000-8000-000000000001",
    requested_by: {
      kind: "user",
      id: "00000000-0000-4000-8000-0000000000a1",
      name: "Ada Lovelace",
    },
    source: "observe",
    trigger: "person",
    prompt: "checkout health",
    status: "succeeded",
    state: "succeeded",
    result_kind: "version",
    allowed_actions: [],
    dashboard_id: null,
    dashboard_title: null,
    panel_id: null,
    result_version: null,
    url: null,
    error: null,
    failure_code: null,
    build: null,
    build_saved_at: null,
    kind: "build",
    title: "Checkout health",
    pinned_at: null,
    continues_id: null,
    continued_by: null,
    retried_by: null,
    continue_note: null,
    observation_id: null,
    observation_title: null,
    chat_id: "chat-1",
    origin: "web",
    origin_client: null,
    created_at: "2026-11-13T10:00:00Z",
    started_at: null,
    finished_at: null,
    ...overrides,
  }
}

const build = {
  saves: 1,
  panels: 4,
  planned_panels: 0,
  title: null,
  updated_at: "2026-11-13T10:00:00Z",
}

describe("chatState", () => {
  test.each([
    ["queued", "Running", "running"],
    ["running", "Running", "running"],
    ["canceling", "Running", "running"],
    ["succeeded", "Done", "ok"],
    ["failed", "Stopped", "warning"],
    ["canceled", "Stopped", "warning"],
  ] as const)("shows %s as %s", (state, word, tone) => {
    expect(chatState(state)).toEqual({ word, tone })
  })
})

describe("statesFor", () => {
  test.each([
    ["running", ["queued", "running", "canceling"]],
    ["stopped", ["failed", "canceled"]],
    ["done", ["succeeded"]],
  ] as const)("lists the states shown as %s", (filter, states) => {
    expect(statesFor(filter)).toEqual(states)
  })
})

describe("chatOutcome", () => {
  test.each([
    [
      "a running build",
      { status: "running", state: "running", build },
      "Building, 4 panels",
    ],
    [
      "a canceling run",
      { status: "running", state: "canceling", build },
      "Canceling",
    ],
    [
      "a queued run",
      { status: "queued", state: "queued", result_kind: "none" },
      "Waiting for the agent",
    ],
    [
      "a failed run with no build",
      { status: "failed", state: "failed", result_kind: "none" },
      "Stopped before anything was built",
    ],
    [
      "a canceled run",
      {
        status: "failed",
        state: "canceled",
        failure_code: "canceled",
        result_kind: "none",
      },
      "Canceled before anything was built",
    ],
    [
      "a new dashboard",
      { source: "observe", dashboard_title: "Checkout" },
      "Saved Checkout",
    ],
    [
      "a revised dashboard",
      { source: "prompt", dashboard_title: "Checkout" },
      "Revised Checkout",
    ],
    [
      "a saved version with no dashboard title",
      { dashboard_title: null, result_version: 3 },
      "Saved version 3",
    ],
    [
      "a partial version",
      { result_kind: "partial_version", dashboard_title: "Checkout" },
      "Saved Checkout",
    ],
    [
      "a stopped run whose build was saved",
      {
        state: "canceled",
        status: "failed",
        failure_code: "canceled",
        result_kind: "partial_version",
        dashboard_title: "Checkout",
      },
      "Saved part of Checkout",
    ],
    [
      "an investigation the caller can see",
      {
        kind: "investigate",
        result_kind: "observation",
        observation_title: "Checkout errors",
      },
      "Answered in Checkout errors",
    ],
    [
      "an investigation the caller cannot see",
      {
        kind: "investigate",
        result_kind: "observation",
        observation_title: null,
      },
      "Saved an observation",
    ],
    [
      "a request for a dashboard that was only answered",
      {
        source: "observe",
        result_kind: "observation",
        observation_title: "Checkout errors",
      },
      "Answered, no dashboard",
    ],
    [
      "a question on a dashboard",
      {
        source: "prompt",
        dashboard_id: "d1",
        result_kind: "observation",
        observation_title: "Checkout errors",
      },
      "Answered, dashboard unchanged",
    ],
    ["a suggestion", { result_kind: "suggestion" }, "Suggested a panel"],
    ["a proposal", { result_kind: "proposal" }, "Opened a proposal"],
    ["nothing", { result_kind: "none" }, "Nothing saved"],
  ] as const)("describes %s", (_name, overrides, text) => {
    expect(chatOutcome(summary(overrides))).toBe(text)
  })

  test("says a stopped run continued elsewhere", () => {
    expect(
      chatOutcome(
        summary({
          status: "failed",
          state: "failed",
          result_kind: "none",
          continued_by: "00000000-0000-4000-8000-000000000002",
        })
      )
    ).toBe("Continued in a new run")
  })
})

describe("deleteChatText", () => {
  test.each([
    [
      "a saved dashboard",
      { result_kind: "version" as const },
      "It leaves your chat lists and cannot be added back. The dashboard it made stays.",
    ],
    [
      "a kept partial build",
      { result_kind: "partial_version" as const },
      "It leaves your chat lists and cannot be added back. The dashboard it made stays.",
    ],
    [
      "an answer in a named observation",
      { result_kind: "observation" as const, observation_title: "p99 latency" },
      'It leaves your chat lists and cannot be added back. The answer stays in the observation "p99 latency", which is not deleted.',
    ],
    [
      "an answer in an observation the reader cannot see",
      { result_kind: "observation" as const, observation_title: null },
      "It leaves your chat lists and cannot be added back. The answer stays in its observation, which is not deleted.",
    ],
    [
      "a suggestion",
      { result_kind: "suggestion" as const },
      "It leaves your chat lists and cannot be added back. The panel suggestion stays.",
    ],
    [
      "a proposal",
      { result_kind: "proposal" as const },
      "It leaves your chat lists and cannot be added back. The proposal stays.",
    ],
    [
      "a run that saved nothing",
      { result_kind: "none" as const },
      "It leaves your chat lists and cannot be added back. Nothing else is removed.",
    ],
  ])("words the dialog for %s", (_name, overrides, expected) => {
    expect(deleteChatText(summary(overrides))).toBe(expected)
  })

  test("never says a dashboard stays for an answer", () => {
    expect(
      deleteChatText(
        summary({ result_kind: "observation", dashboard_id: null })
      )
    ).not.toMatch(/dashboard/i)
  })
})
