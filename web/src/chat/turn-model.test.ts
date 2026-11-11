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
    ["a kept stopped build", "partial_version", failedAt, savedAt, savedAt],
    ["a stopped build that was not kept", "none", failedAt, null, failedAt],
    ["a request that saved a version", "version", failedAt, null, failedAt],
    [
      "a request that saved a version ignores build_saved_at",
      "version",
      failedAt,
      savedAt,
      failedAt,
    ],
    ["a request still running", "none", null, null, null],
  ] as const)(
    "%s",
    (_name, result_kind, finished_at, build_saved_at, expected) => {
      expect(
        resultFinishedAt({ result_kind, finished_at, build_saved_at })
      ).toBe(expected)
    }
  )
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
    ["an investigation", "investigate", "none", true],
    [
      "an investigation that saved a finding",
      "investigate",
      "observation",
      true,
    ],
    ["a new dashboard that answered", "observe", "observation", true],
    ["a new dashboard that saved a version", "observe", "version", false],
    ["a new dashboard still running", "observe", "none", false],
    ["a failed new dashboard", "observe", "none", false],
    ["a stopped build that was kept", "observe", "partial_version", false],
    ["a dashboard chat prompt that answered", "prompt", "observation", true],
    ["a comment that answered", "comment", "observation", true],
    ["a revision that saved a version", "prompt", "version", false],
    ["a revision still running", "prompt", "none", false],
  ] as const)("%s", (_name, source, result_kind, expected) => {
    expect(mayHaveFinding({ source, result_kind })).toBe(expected)
  })
})
