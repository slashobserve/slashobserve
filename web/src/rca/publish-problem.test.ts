import { describe, expect, it } from "vitest"

import { ApiError, NetworkError, unreachableText } from "@/api/errors"

import type { Block } from "@/observations/queries"

import { publishProblem } from "./publish-problem"

function failure(
  status: number,
  code: string,
  message: string,
  issues: { path: string; code: string; message: string }[] = []
) {
  return new ApiError(status, code, message, issues)
}

describe("publishProblem", () => {
  const cases: [string, unknown, string][] = [
    [
      "the revision limit",
      failure(409, "rca_limit", "too many"),
      "This observation has 50 revisions, the most it can have.",
    ],
    [
      "a page that changed while publishing",
      failure(409, "observation_changed", "changed"),
      "The page changed while it was being published. Publish again.",
    ],
    [
      "a panel that could not be frozen",
      failure(422, "panel_failed", "a panel failed", [
        {
          path: "/blocks/2",
          code: "query_timeout",
          message: "the query ran out of time",
        },
        { path: "/blocks/3", code: "internal", message: "second" },
      ]),
      "A panel could not be frozen: the query ran out of time",
    ],
    [
      "a panel failure without issues",
      failure(422, "panel_failed", "a panel failed"),
      "A panel failed.",
    ],
    [
      "frozen data that is too large",
      failure(422, "snapshot_too_large", "over 2 MiB"),
      "A panel returned too much data to freeze. Narrow its time range and publish again.",
    ],
    [
      "a state that does not allow publishing",
      failure(409, "invalid_transition", "this observation is not open"),
      "This observation is not open.",
    ],
    [
      "a personal folder",
      failure(
        409,
        "personal_folder",
        "observations in My Dashboards stay private"
      ),
      "Observations in My Dashboards stay private.",
    ],
    [
      "an empty server message",
      failure(500, "internal", ""),
      "Could not publish",
    ],
    ["a network failure", new NetworkError("offline"), unreachableText],
  ]

  it.each(cases)("words %s", (_name, error, expected) => {
    expect(publishProblem(error).text).toBe(expected)
  })

  describe("a panel that could not be frozen", () => {
    const blocks = [
      { id: "b0", position: 0, kind: "text", body: { text: "hi" } },
      {
        id: "b2",
        position: 2,
        kind: "panel",
        body: { panel: { title: "Checkout latency" } },
      },
    ] as unknown as Block[]
    const error = failure(422, "panel_failed", "a panel failed", [
      { path: "/blocks/2", code: "query_timeout", message: "ran out of time" },
    ])

    it("names the block the issue points at and returns its id", () => {
      expect(publishProblem(error, blocks)).toEqual({
        text: 'The panel "Checkout latency" could not be frozen: ran out of time',
        blockId: "b2",
      })
    })

    it("keeps the generic wording when no block has that position", () => {
      expect(publishProblem(error, [blocks[0]])).toEqual({
        text: "A panel could not be frozen: ran out of time",
      })
    })
  })
})

describe("publishProblem unchanged page", () => {
  it("passes the server's reason through as a sentence", () => {
    expect(
      publishProblem(
        new ApiError(
          409,
          "rca_unchanged",
          "revision 2 already holds this page; change the observation before publishing again"
        )
      ).text
    ).toBe(
      "Revision 2 already holds this page; change the observation before publishing again."
    )
  })
})
