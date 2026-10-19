import { describe, expect, it } from "vitest"

import { ApiError, maxRetryDelayMs } from "@/api/errors"

import {
  agentOffMessage,
  describeSubmitError,
  pollDelayMs,
  pollFailures,
  requestStatusView,
  sentence,
  stopsPolling,
} from "./queries"

describe("sentence", () => {
  it("capitalizes and ends with a period", () => {
    expect(sentence("prompt is too long")).toBe("Prompt is too long.")
  })

  it("keeps existing terminal punctuation", () => {
    expect(sentence("Nope!")).toBe("Nope!")
    expect(sentence("really?")).toBe("Really?")
  })

  it("trims trailing whitespace and returns empty text unchanged", () => {
    expect(sentence("done  \n")).toBe("Done.")
    expect(sentence("  ")).toBe("")
  })
})

describe("describeSubmitError", () => {
  it("shows a 400 message inline as invalid input", () => {
    const error = new ApiError(400, "invalid_prompt", "prompt is required")
    expect(describeSubmitError(error)).toEqual({
      message: "Prompt is required.",
      inline: true,
      invalid: true,
    })
  })

  it("uses the server's caller limit message", () => {
    const error = new ApiError(
      429,
      "too_many_requests",
      "you already have 5 new dashboard requests queued or running"
    )
    expect(describeSubmitError(error)).toEqual({
      message:
        "You already have 5 new dashboard requests queued or running. Try again when one finishes.",
      inline: true,
      invalid: false,
    })
  })

  it("uses the server's message for the per-dashboard revision limit", () => {
    const error = new ApiError(
      429,
      "too_many_revisions",
      "this dashboard already has 5 revisions queued or running"
    )
    expect(describeSubmitError(error)).toEqual({
      message:
        "This dashboard already has 5 revisions queued or running. Try again when one finishes.",
      inline: true,
      invalid: false,
    })
  })

  it("tells busy and generic 429 apart", () => {
    const busy = describeSubmitError(new ApiError(429, "agent_busy", "busy"))
    const other = describeSubmitError(new ApiError(429, "unknown", "slow down"))
    expect(busy.message).toContain("agent is busy")
    expect(other.message).toContain("Too many requests")
    expect(busy.inline).toBe(true)
    expect(other.inline).toBe(true)
  })

  it("explains an unavailable agent inline", () => {
    const error = new ApiError(503, "agent_unavailable", "no provider")
    expect(describeSubmitError(error)).toEqual({
      message: agentOffMessage,
      inline: true,
      invalid: false,
    })
  })

  it("sends other API errors to a toast", () => {
    const error = new ApiError(500, "internal", "internal error")
    expect(describeSubmitError(error)).toEqual({
      message: "Internal error.",
      inline: false,
      invalid: false,
    })
  })

  it("handles network and unknown errors", () => {
    expect(describeSubmitError(new Error("Failed to fetch")).message).toBe(
      "Failed to fetch."
    )
    expect(describeSubmitError("boom")).toEqual({
      message: "The request could not be sent.",
      inline: false,
      invalid: false,
    })
  })
})

describe("stopsPolling", () => {
  it("stops on client errors that retrying cannot fix", () => {
    for (const status of [400, 401, 403, 404]) {
      expect(stopsPolling(new ApiError(status, "x", "x"))).toBe(true)
    }
  })

  it("keeps polling through server errors, throttling and non-API errors", () => {
    for (const status of [429, 500, 502, 503]) {
      expect(stopsPolling(new ApiError(status, "x", "x"))).toBe(false)
    }
    expect(stopsPolling(new Error("offline"))).toBe(false)
    expect(stopsPolling(null)).toBe(false)
  })
})

describe("pollDelayMs", () => {
  it("polls every second while healthy", () => {
    expect(pollDelayMs(null, 0)).toBe(1000)
  })

  it("backs off from one second and caps at eight", () => {
    const error = new ApiError(503, "x", "x")
    expect([1, 2, 3, 4, 5, 9].map((n) => pollDelayMs(error, n))).toEqual([
      1000, 2000, 4000, 8000, 8000, 8000,
    ])
  })

  it("honours Retry-After within bounds", () => {
    const at = (ms: number) => new ApiError(429, "x", "x", [], undefined, ms)
    expect(pollDelayMs(at(5000), 1)).toBe(5000)
    expect(pollDelayMs(at(100), 1)).toBe(1000)
    expect(pollDelayMs(at(10 * maxRetryDelayMs), 1)).toBe(maxRetryDelayMs)
  })
})

describe("pollFailures", () => {
  it("counts failed polls since the last success", () => {
    const query = { state: { dataUpdateCount: 0, errorUpdateCount: 0 } }
    expect(pollFailures(query)).toBe(0)
    query.state = { dataUpdateCount: 1, errorUpdateCount: 0 }
    expect(pollFailures(query)).toBe(0)
    query.state = { dataUpdateCount: 1, errorUpdateCount: 2 }
    expect(pollFailures(query)).toBe(2)
    expect(pollFailures(query)).toBe(2)
    query.state = { dataUpdateCount: 2, errorUpdateCount: 2 }
    expect(pollFailures(query)).toBe(0)
    query.state = { dataUpdateCount: 2, errorUpdateCount: 3 }
    expect(pollFailures(query)).toBe(1)
  })

  it("counts failures before the first success", () => {
    const query = { state: { dataUpdateCount: 0, errorUpdateCount: 1 } }
    expect(pollFailures(query)).toBe(1)
  })
})

describe("requestStatusView", () => {
  const base = { dashboard_title: null, error: null, result_version: null }

  it("shows queued and running requests as working", () => {
    expect(requestStatusView({ ...base, status: "queued" })).toEqual({
      tone: "running",
      text: "Working",
    })
    expect(requestStatusView({ ...base, status: "running" })).toEqual({
      tone: "running",
      text: "Working",
    })
  })

  it("shows a succeeded request with the dashboard title", () => {
    expect(
      requestStatusView({
        ...base,
        status: "succeeded",
        dashboard_title: "Checkout health",
        result_version: 1,
      })
    ).toEqual({ tone: "ok", text: "Checkout health" })
  })

  it("falls back to a generic saved text without a title", () => {
    expect(requestStatusView({ ...base, status: "succeeded" })).toEqual({
      tone: "ok",
      text: "Dashboard saved",
    })
  })

  it("shows a failed request with the error as a sentence", () => {
    expect(
      requestStatusView({ ...base, status: "failed", error: "model timed out" })
    ).toEqual({ tone: "critical", text: "Model timed out." })
  })

  it("falls back to a generic failure text without an error", () => {
    expect(requestStatusView({ ...base, status: "failed" })).toEqual({
      tone: "critical",
      text: "The request failed.",
    })
  })
})
