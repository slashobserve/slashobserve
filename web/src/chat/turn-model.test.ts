import { describe, expect, it } from "vitest"

import { resultFinishedAt, showsWorkingRow } from "./turn-model"

const failedAt = "2026-11-04T10:00:00Z"
const savedAt = "2026-11-04T10:00:23Z"

describe("resultFinishedAt", () => {
  it.each([
    ["a saved stopped build", "failed", failedAt, savedAt, savedAt],
    ["a stopped build that was not saved", "failed", failedAt, null, failedAt],
    ["a succeeded request", "succeeded", failedAt, null, failedAt],
    [
      "a succeeded request ignores build_saved_at",
      "succeeded",
      failedAt,
      savedAt,
      failedAt,
    ],
    ["a request still running", "running", null, null, null],
  ] as const)("%s", (_name, status, finished_at, build_saved_at, expected) => {
    expect(resultFinishedAt({ status, finished_at, build_saved_at })).toBe(
      expected
    )
  })
})

describe("showsWorkingRow", () => {
  it.each([
    ["queued", false, false],
    ["queued", true, false],
    ["running", false, true],
    ["running", true, false],
  ] as const)("%s, stalled %s", (status, stalled, expected) => {
    expect(showsWorkingRow({ status }, stalled)).toBe(expected)
  })
})
