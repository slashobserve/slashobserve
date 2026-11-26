import { describe, expect, it } from "vitest"

import { ApiError } from "@/api/errors"

import {
  failedWithoutData,
  failureOf,
  staleNotice,
  staleNoticeOf,
  type FailingQuery,
} from "./panel-stale"

const at = Date.UTC(2026, 10, 26, 9, 5, 30)
const sameDay = Date.UTC(2026, 10, 26, 18, 0, 0)

function query(patch: Partial<FailingQuery>): FailingQuery {
  return {
    data: undefined,
    isError: false,
    failureCount: 0,
    error: null,
    failureReason: null,
    dataUpdatedAt: at,
    ...patch,
  }
}

describe("staleNotice", () => {
  it("names the time of the old data in the zone and the reason", () => {
    expect(
      staleNotice(at, new Error("upstream timed out"), "UTC", sameDay)
    ).toBe(
      "Showing data from 09:05; the last refresh failed: upstream timed out"
    )
    expect(
      staleNotice(at, new Error("upstream timed out"), "Asia/Kolkata", sameDay)
    ).toBe(
      "Showing data from 14:35; the last refresh failed: upstream timed out"
    )
  })

  it("words an API error with its query", () => {
    const error = new ApiError(
      502,
      "datasource_unavailable",
      "Connection refused",
      [],
      undefined,
      undefined,
      "A"
    )
    expect(staleNotice(at, error, "UTC", sameDay)).toContain(
      "the last refresh failed: Query A failed: connection refused"
    )
  })
})

describe("staleNotice, dates and a missing update time", () => {
  const now = Date.UTC(2026, 10, 26, 18, 0, 0)

  it("adds the date when the data is from another day in the zone", () => {
    expect(staleNotice(at, new Error("boom"), "UTC", now)).toBe(
      "Showing data from 09:05; the last refresh failed: boom"
    )
    expect(
      staleNotice(Date.UTC(2026, 10, 25, 23, 30), new Error("boom"), "UTC", now)
    ).toBe("Showing data from 2026-11-25 23:30; the last refresh failed: boom")
  })

  it("compares the day in the panel's zone, not in UTC", () => {
    expect(
      staleNotice(
        Date.UTC(2026, 10, 25, 23, 30),
        new Error("boom"),
        "Asia/Kolkata",
        Date.UTC(2026, 10, 26, 3, 0)
      )
    ).toBe("Showing data from 05:00; the last refresh failed: boom")
  })

  it("does not print a time for data with no update time", () => {
    expect(staleNotice(0, new Error("boom"), "UTC", now)).toBe(
      "Showing earlier data; the last refresh failed: boom"
    )
  })
})

describe("staleNoticeOf", () => {
  it("speaks only when a failed refresh left old data on the panel", () => {
    const error = new Error("boom")
    expect(
      staleNoticeOf(query({ data: {}, isError: true, error }), undefined, "UTC")
    ).toMatch(
      /^Showing data from (\d{4}-\d{2}-\d{2} )?09:05; the last refresh failed: boom$/
    )
    expect(staleNoticeOf(query({ isError: true, error }), "UTC", "UTC")).toBe(
      undefined
    )
    expect(staleNoticeOf(query({ data: {} }), "UTC", "UTC")).toBe(undefined)
  })
})

describe("failedWithoutData", () => {
  it("is true after the first failed attempt, while retries are still running", () => {
    expect(failedWithoutData(query({ failureCount: 1 }))).toBe(true)
    expect(failedWithoutData(query({ isError: true }))).toBe(true)
  })

  it("is false while loading for the first time and whenever there is data", () => {
    expect(failedWithoutData(query({}))).toBe(false)
    expect(failedWithoutData(query({ data: {}, isError: true }))).toBe(false)
  })
})

describe("failureOf", () => {
  it("falls back to the reason of the attempt that failed while retrying", () => {
    const reason = new Error("first attempt")
    expect(failureOf(query({ failureReason: reason }))).toBe(reason)
    const error = new Error("gave up")
    expect(failureOf(query({ error, failureReason: reason }))).toBe(error)
  })
})
