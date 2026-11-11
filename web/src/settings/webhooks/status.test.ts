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
      })
    ).toEqual({ tone: "ok", label: "Sending", detail: null })
  })

  test("a webhook the server marks retrying says how many attempts failed", () => {
    expect(
      webhookStatus({
        state: "retrying",
        disabled_reason: null,
        consecutive_failures: 1,
      })
    ).toEqual({
      tone: "warning",
      label: "Retrying",
      detail: "1 attempt in a row failed",
    })
    expect(
      webhookStatus({
        state: "retrying",
        disabled_reason: null,
        consecutive_failures: 4,
      }).detail
    ).toBe("4 attempts in a row failed")
  })

  test("a disabled webhook says why", () => {
    expect(
      webhookStatus({
        state: "disabled",
        disabled_reason: "gone",
        consecutive_failures: 3,
      })
    ).toEqual({
      tone: "critical",
      label: "Disabled",
      detail: "The receiver answered 410 Gone",
    })
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
    expect(errorText("something_new")).toBe("something_new")
    expect(outcomeTone("delivered")).toBe("ok")
    expect(outcomeTone("skipped")).toBe("warning")
    expect(outcomeTone("failed")).toBe("critical")
  })
})
