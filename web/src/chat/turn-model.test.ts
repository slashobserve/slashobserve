import { describe, expect, it } from "vitest"

import {
  formatRunDuration,
  mayHaveFinding,
  resultFinishedAt,
  showsWorkingRow,
  tookText,
  turnDurationMs,
} from "./turn-model"

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

describe("turnDurationMs", () => {
  const start = "2026-11-04T10:00:00Z"
  const now = Date.parse("2026-11-04T10:01:12Z")

  it.each([
    ["no start", null, null, null],
    ["a finished run", start, "2026-11-04T10:00:08Z", 8000],
    ["a running run against now", start, null, 72_000],
    ["an unparsable start", "not a time", null, null],
    ["an unparsable finish", start, "later", null],
    ["a finish before the start", start, "2026-11-04T09:59:00Z", 0],
  ])("%s", (_name, startedAt, finishedAt, expected) => {
    expect(turnDurationMs(startedAt, finishedAt, now)).toBe(expected)
  })
})

describe("formatRunDuration", () => {
  it.each([
    [0, "0s"],
    [8_400, "8s"],
    [59_999, "59s"],
    [60_000, "1m 0s"],
    [72_000, "1m 12s"],
    [3_599_000, "59m 59s"],
    [3_780_000, "1h 3m"],
  ])("formats %d ms as %s", (ms, expected) => {
    expect(formatRunDuration(ms)).toBe(expected)
  })
})

describe("tookText", () => {
  it("formats a finished run and skips an unfinished or unstarted one", () => {
    expect(tookText("2026-11-04T10:00:00Z", "2026-11-04T10:01:12Z")).toBe(
      "1m 12s"
    )
    expect(tookText("2026-11-04T10:00:00Z", null)).toBeUndefined()
    expect(tookText(null, "2026-11-04T10:01:12Z")).toBeUndefined()
  })
})

describe("mayHaveFinding", () => {
  it.each([
    ["an investigation", "investigate", "running", null, true],
    ["a new dashboard that answered", "observe", "succeeded", null, true],
    ["a new dashboard that saved a version", "observe", "succeeded", 1, false],
    ["a new dashboard still running", "observe", "running", null, false],
    ["a failed new dashboard", "observe", "failed", null, false],
    [
      "a dashboard chat prompt that answered",
      "prompt",
      "succeeded",
      null,
      true,
    ],
    ["a comment that answered", "comment", "succeeded", null, true],
    ["a revision that saved a version", "prompt", "succeeded", 4, false],
    ["a revision still running", "prompt", "running", null, false],
  ] as const)("%s", (_name, source, status, result_version, expected) => {
    expect(mayHaveFinding({ source, status, result_version })).toBe(expected)
  })
})
