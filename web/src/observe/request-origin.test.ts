import { describe, expect, it } from "vitest"

import { requestOrigin, type OriginFields } from "./request-origin"

const revision: OriginFields = {
  kind: "build",
  intent: "apply",
  base_version: 3,
  comment_id: null,
}

describe("requestOrigin", () => {
  it.each<[string, Partial<OriginFields>, string]>([
    [
      "a new dashboard before it saves",
      { intent: "ask", base_version: null },
      "observe",
    ],
    ["a new dashboard that saved", { base_version: null }, "observe"],
    ["a dashboard chat revision", {}, "prompt"],
    ["a revision asked in a comment thread", { comment_id: "c1" }, "comment"],
    ["an investigation", { kind: "investigate", intent: "ask" }, "investigate"],
    [
      "a private ask about a dashboard",
      { kind: "investigate", intent: "ask", base_version: 3 },
      "investigate",
    ],
    ["a revision row with no kind", { kind: undefined }, "prompt"],
  ])("tells %s", (_name, overrides, expected) => {
    expect(requestOrigin({ ...revision, ...overrides })).toBe(expected)
  })
})
