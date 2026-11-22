import { describe, expect, test } from "vitest"

import {
  type AuditEvent,
  auditStreamUrl,
  forbiddenText,
  liveRowLimit,
  parseAuditFrame,
  stopFrameText,
  streamRefusal,
  withLiveEvent,
} from "./stream"

function event(id: string): AuditEvent {
  return { id, occurred_at: "2026-11-21T06:00:00Z" } as AuditEvent
}

describe("auditStreamUrl", () => {
  test("follows every event without a query", () => {
    expect(auditStreamUrl("  ")).toBe("/api/v1/audit/stream")
  })

  test("sends the query as q", () => {
    expect(auditStreamUrl("outcome:denied and actor:Zoë")).toBe(
      "/api/v1/audit/stream?q=outcome%3Adenied+and+actor%3AZo%C3%AB"
    )
  })
})

describe("parseAuditFrame", () => {
  test("reads an event", () => {
    expect(
      parseAuditFrame('{"id":"e1","occurred_at":"2026-11-21T06:00:00Z"}')
    ).toMatchObject({ id: "e1" })
  })

  test("drops a frame that is not an event", () => {
    expect(parseAuditFrame("{}")).toBe(null)
    expect(parseAuditFrame("not json")).toBe(null)
    expect(parseAuditFrame("[]")).toBe(null)
  })
})

describe("stopFrameText", () => {
  test("explains a forbidden frame", () => {
    expect(stopFrameText("forbidden", '{"code":"forbidden_role"}')).toBe(
      forbiddenText
    )
  })

  test("explains a query that is too expensive to follow", () => {
    expect(
      stopFrameText("error", '{"error":"slow","code":"query_too_expensive"}')
    ).toMatch(/too slow to follow/)
  })

  test("passes another error through", () => {
    expect(stopFrameText("error", '{"error":"boom","code":"internal"}')).toBe(
      "Live stopped: boom"
    )
  })
})

describe("streamRefusal", () => {
  test("stops on too many streams, a refused role and a bad query", () => {
    expect(streamRefusal(429, "too_many_streams", "")).toMatchObject({
      kind: "stop",
      message: expect.stringMatching(/already follow 5 audit streams/),
    })
    expect(streamRefusal(403, "forbidden_role", "")).toEqual({
      kind: "stop",
      message: forbiddenText,
    })
    expect(streamRefusal(422, "query_invalid", 'unexpected ")"')).toEqual({
      kind: "stop",
      message: 'Live stopped: unexpected ")"',
    })
  })

  test("starts over after a refused resume", () => {
    expect(streamRefusal(400, "invalid_request", "")).toEqual({ kind: "gap" })
  })

  test("retries a server that is shutting down", () => {
    expect(streamRefusal(503, "shutting_down", "")).toEqual({ kind: "retry" })
    expect(streamRefusal(500, "internal", "")).toEqual({ kind: "retry" })
  })
})

describe("withLiveEvent", () => {
  const anchor = Date.parse("2026-11-21T06:00:00Z")
  const later = Date.parse("2026-11-21T06:10:00Z")

  function at(id: string, time: string): AuditEvent {
    return { id, occurred_at: time } as AuditEvent
  }

  test("puts a new event on top", () => {
    const next = withLiveEvent(
      { anchor, live: [event("a")] },
      event("b"),
      new Set(),
      later
    )
    expect(next.anchor).toBe(anchor)
    expect(next.live.map((row) => row.id)).toEqual(["b", "a"])
  })

  test("skips an event already shown", () => {
    const current = { anchor, live: [event("a")] }
    expect(withLiveEvent(current, event("a"), new Set(), later)).toBe(current)
    expect(withLiveEvent(current, event("p"), new Set(["p"]), later)).toBe(
      current
    )
  })

  test("past the live row limit, moves the anchor to now and keeps only rows after it", () => {
    const live = Array.from({ length: liveRowLimit }, (_, index) =>
      event(`e${index}`)
    )
    const ahead = at("ahead", "2026-11-21T06:10:00.500Z")
    const next = withLiveEvent({ anchor, live }, ahead, new Set(), later)
    expect(next).toEqual({ anchor: later, live: [ahead] })
  })
})
