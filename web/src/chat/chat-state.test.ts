import { describe, expect, test } from "vitest"

import type { RequestSummary } from "@/observe/queries"

import {
  chatOutcome,
  chatState,
  deleteChatText,
  deleteObservationLabel,
  deleteTurnsText,
  savedObservations,
  statesFor,
} from "./chat-state"

function summary(overrides: Partial<RequestSummary> = {}): RequestSummary {
  return {
    id: "00000000-0000-4000-8000-000000000001",
    requested_by: {
      kind: "user",
      id: "00000000-0000-4000-8000-0000000000a1",
      name: "Ada Lovelace",
    },
    anchor: null,
    block_id: null,
    intent: "apply",
    investigation: false,
    base_version: null,
    comment_id: null,
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
      { intent: "apply", base_version: null, dashboard_title: "Checkout" },
      "Saved Checkout",
    ],
    [
      "a revised dashboard",
      { intent: "apply", base_version: 1, dashboard_title: "Checkout" },
      "Revised Checkout",
    ],
    [
      "a saved version with no dashboard",
      { dashboard_title: null, result_version: 3 },
      "Saved version 3",
    ],
    [
      "a new dashboard whose title the caller cannot read",
      {
        intent: "apply",
        base_version: null,
        dashboard_id: "d1",
        dashboard_title: null,
      },
      "Saved a dashboard",
    ],
    [
      "a revision whose dashboard the caller cannot read",
      {
        intent: "apply",
        base_version: 1,
        dashboard_id: "d1",
        dashboard_title: null,
        result_version: 3,
      },
      "Revised a dashboard",
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
      "an investigation that had no observation to write into",
      {
        kind: "investigate",
        result_kind: "answer",
        observation_id: null,
        observation_title: null,
      },
      "Answered, no observation",
    ],
    [
      "a request for a dashboard that was only answered",
      {
        intent: "apply",
        base_version: null,
        result_kind: "observation",
        observation_title: "Checkout errors",
      },
      "Answered in Checkout errors",
    ],
    [
      "an answer to a question on Home saved as an observation",
      {
        intent: "ask",
        result_kind: "answer",
        observation_id: "o1",
        observation_title: "Checkout errors",
      },
      "Answered in Checkout errors",
    ],
    [
      "a request for a dashboard whose answer was not saved",
      { intent: "apply", base_version: null, result_kind: "answer" },
      "Answered, no dashboard",
    ],
    [
      "a question on a dashboard",
      {
        intent: "apply",
        base_version: 1,
        dashboard_id: "d1",
        result_kind: "observation",
        observation_title: "Checkout errors",
      },
      "Answered in Checkout errors, dashboard unchanged",
    ],
    [
      "a question on a dashboard whose answer was not saved",
      {
        intent: "apply",
        base_version: 1,
        dashboard_id: "d1",
        result_kind: "answer",
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
      {
        result_kind: "observation" as const,
        observation_id: "o1",
        observation_title: "p99 latency",
      },
      'It leaves your chat lists and cannot be added back. The answer stays in the observation "p99 latency", which is not deleted.',
    ],
    [
      "an answer in an observation the reader cannot see",
      {
        result_kind: "observation" as const,
        observation_id: "o1",
        observation_title: null,
      },
      "It leaves your chat lists and cannot be added back. The answer stays in its observation, which is not deleted.",
    ],
    [
      "an answer kept in the chat",
      { result_kind: "answer" as const },
      "It leaves your chat lists and cannot be added back. The answer stays on the chat page, which still opens from its link.",
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
      deleteChatText(summary({ result_kind: "answer", dashboard_id: null }))
    ).not.toMatch(/dashboard/i)
  })
})

describe("deleteTurnsText", () => {
  const left =
    "All 3 messages in this chat leave your chat lists and cannot be added back."

  function turns(...overrides: Partial<RequestSummary>[]): RequestSummary[] {
    return overrides.map((turn, index) => summary({ id: `t${index}`, ...turn }))
  }

  test("words one message as a single chat", () => {
    expect(deleteTurnsText(turns({ result_kind: "none" }))).toBe(
      deleteChatText(summary({ id: "t0", result_kind: "none" }))
    )
  })

  test("says nothing else is removed when no message saved anything", () => {
    expect(
      deleteTurnsText(
        turns(
          { result_kind: "none" },
          { result_kind: "none" },
          {
            result_kind: "none",
          }
        )
      )
    ).toBe(`${left} Nothing else is removed.`)
  })

  test("names a dashboard once however many messages revised it", () => {
    const dashboard = {
      result_kind: "version" as const,
      dashboard_id: "d1",
      dashboard_title: "Checkout health",
    }
    expect(
      deleteTurnsText(turns(dashboard, dashboard, { result_kind: "none" }))
    ).toBe(`${left} What they saved stays: the dashboard "Checkout health".`)
  })

  test("names the dashboard, the observation and the proposal", () => {
    expect(
      deleteTurnsText(
        turns(
          {
            result_kind: "version",
            dashboard_id: "d1",
            dashboard_title: "Checkout health",
          },
          {
            result_kind: "observation",
            observation_id: "o1",
            observation_title: "Slowest routes",
          },
          { result_kind: "proposal" }
        )
      )
    ).toBe(
      `${left} What they saved stays: the dashboard "Checkout health", the observation "Slowest routes" and a proposal.`
    )
  })

  test("does not name what the reader cannot see", () => {
    expect(
      deleteTurnsText(
        turns(
          { result_kind: "version", dashboard_id: "d1", dashboard_title: null },
          {
            result_kind: "observation",
            observation_id: "o1",
            observation_title: null,
          },
          { result_kind: "suggestion" }
        )
      )
    ).toBe(
      `${left} What they saved stays: a dashboard, an observation and a panel suggestion.`
    )
  })

  test("says where an unsaved answer stays", () => {
    expect(
      deleteTurnsText(
        turns(
          { result_kind: "answer" },
          { result_kind: "none" },
          { result_kind: "none" }
        )
      )
    ).toBe(`${left} What they saved stays: an answer on its chat page.`)
  })

  test("says the observation chosen for removal is deleted, not kept", () => {
    expect(
      deleteTurnsText(
        turns(
          {
            result_kind: "version",
            dashboard_id: "d1",
            dashboard_title: "Checkout health",
          },
          {
            result_kind: "observation",
            observation_id: "o1",
            observation_title: "Slowest routes",
          },
          { result_kind: "none" }
        ),
        new Set(["o1"])
      )
    ).toBe(
      `${left} What they saved stays: the dashboard "Checkout health". The observation "Slowest routes" is deleted for everyone, with its blocks, discussion and published RCA revisions, and this cannot be undone.`
    )
  })

  test("names every removed observation when nothing else stays", () => {
    expect(
      deleteTurnsText(
        turns(
          {
            result_kind: "observation",
            observation_id: "o1",
            observation_title: "Slowest routes",
          },
          {
            result_kind: "observation",
            observation_id: "o2",
            observation_title: "Error spike",
          },
          { result_kind: "none" }
        ),
        new Set(["o1", "o2"])
      )
    ).toBe(
      `${left} The observations "Slowest routes" and "Error spike" are deleted for everyone, with their blocks, discussion and published RCA revisions, and this cannot be undone.`
    )
  })
})

describe("deleting the saved observation with the chat", () => {
  const answer = summary({
    result_kind: "observation",
    observation_id: "o1",
    observation_title: "Slowest routes",
  })

  test("keeps today's wording while the observation is not chosen", () => {
    expect(deleteChatText(answer, new Set())).toBe(
      'It leaves your chat lists and cannot be added back. The answer stays in the observation "Slowest routes", which is not deleted.'
    )
  })

  test("says a single chat's observation is deleted when chosen", () => {
    expect(deleteTurnsText([answer], new Set(["o1"]))).toBe(
      'It leaves your chat lists and cannot be added back. The observation "Slowest routes" is deleted for everyone, with its blocks, discussion and published RCA revisions, and this cannot be undone.'
    )
  })

  test("offers only observations the reader can see, once each", () => {
    expect(
      savedObservations([
        answer,
        answer,
        summary({
          result_kind: "observation",
          observation_id: "o2",
          observation_title: null,
        }),
        summary({ result_kind: "answer" }),
        summary({ result_kind: "version", observation_id: "o3" }),
      ])
    ).toEqual([{ id: "o1", title: "Slowest routes" }])
  })

  test("names the observation on the checkbox", () => {
    expect(deleteObservationLabel({ id: "o1", title: "Slowest routes" })).toBe(
      'Also delete the observation "Slowest routes"'
    )
  })
})
