import { describe, expect, it } from "vitest"

import { ApiError, NetworkError } from "@/api/errors"

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
    ["a network failure", new NetworkError("offline"), "Could not publish"],
  ]

  it.each(cases)("words %s", (_name, error, expected) => {
    expect(publishProblem(error)).toBe(expected)
  })
})
