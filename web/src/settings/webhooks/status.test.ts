import { describe, expect, test } from "vitest"

import {
  deliverySummary,
  errorText,
  outcomeTone,
  webhookStatus,
} from "./status"

describe("webhookStatus", () => {
  test("a healthy webhook is sending", () => {
    expect(
      webhookStatus({
        state: "sending",
        disabled_reason: null,
        consecutive_failures: 0,
        waiting_events: null,
      })
    ).toEqual({ tone: "ok", label: "Sending", detail: null, waiting: null })
  })

  test("a webhook the server marks retrying says how many attempts failed", () => {
    expect(
      webhookStatus({
        state: "retrying",
        disabled_reason: null,
        consecutive_failures: 1,
        waiting_events: null,
      })
    ).toEqual({
      tone: "warning",
      label: "Retrying",
      detail: "1 attempt in a row failed",
      waiting: null,
    })
    expect(
      webhookStatus({
        state: "retrying",
        disabled_reason: null,
        consecutive_failures: 4,
        waiting_events: null,
      }).detail
    ).toBe("4 attempts in a row failed")
  })

  test("a retrying webhook counts the events stuck behind the failing one", () => {
    const status = (waiting_events: { count: number; more: boolean } | null) =>
      webhookStatus({
        state: "retrying",
        disabled_reason: null,
        consecutive_failures: 21,
        waiting_events,
      }).waiting
    expect(status({ count: 1, more: false })).toBe("1 event waiting")
    expect(status({ count: 3, more: false })).toBe("3 events waiting")
    expect(status({ count: 100, more: true })).toBe(
      "100 or more events waiting"
    )
    expect(status({ count: 0, more: false })).toBeNull()
    expect(status(null)).toBeNull()
  })

  test("a disabled webhook says why", () => {
    expect(
      webhookStatus({
        state: "disabled",
        disabled_reason: "gone",
        consecutive_failures: 3,
        waiting_events: null,
      })
    ).toEqual({
      tone: "critical",
      label: "Disabled",
      detail: "The receiver answered 410 Gone",
      waiting: null,
    })
  })

  test("a webhook an Admin turned off is neutral, not an alert", () => {
    expect(
      webhookStatus({
        state: "disabled",
        disabled_reason: "manual",
        consecutive_failures: 0,
        waiting_events: null,
      }).tone
    ).toBe("none")
  })
})

describe("deliverySummary", () => {
  test("a delivered attempt names its status and time", () => {
    expect(
      deliverySummary({
        outcome: "delivered",
        status_code: 200,
        duration_ms: 120,
        error_code: null,
      })
    ).toBe("Delivered: status 200, 120 ms")
  })

  test("a failed attempt names the error, and the status when there is one", () => {
    expect(
      deliverySummary({
        outcome: "failed",
        status_code: 503,
        duration_ms: 40,
        error_code: "status",
      })
    ).toBe(
      "Not delivered: The receiver answered with an error status (status 503, 40 ms)"
    )
    expect(
      deliverySummary({
        outcome: "failed",
        status_code: null,
        duration_ms: 7,
        error_code: "unreachable",
      })
    ).toBe("Not delivered: The receiver could not be reached (7 ms)")
  })
})

describe("errorText and outcomeTone", () => {
  test("map codes and outcomes", () => {
    expect(errorText(null)).toBe("-")
    expect(errorText("too_large")).toBe("The event is larger than 64 KiB")
    expect(errorText("expired")).toBe("The event was more than 24 hours old")
    expect(errorText("something_new")).toBe("something_new")
    expect(outcomeTone("delivered")).toBe("ok")
    expect(outcomeTone("skipped")).toBe("warning")
    expect(outcomeTone("failed")).toBe("critical")
  })
})
