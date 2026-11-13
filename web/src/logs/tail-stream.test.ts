import { describe, expect, it } from "vitest"

import {
  parseTailEvent,
  tailFailureText,
  tailStartFrom,
  tailUrl,
} from "./tail-stream"

describe("tailUrl", () => {
  it("encodes a Loki selector with braces, quotes and a pipe", () => {
    const url = tailUrl(
      "loki main",
      '{service_name="frontend"} |= "a b"',
      "2026-11-07T12:00:00.123Z",
      "Asia/Kolkata"
    )
    const [path, query] = url.split("?")
    expect(path).toBe("/api/v1/datasources/loki%20main/tail")
    const params = new URLSearchParams(query)
    expect(params.get("query")).toBe('{service_name="frontend"} |= "a b"')
    expect(params.get("from")).toBe("2026-11-07T12:00:00.123Z")
    expect(params.get("limit")).toBe("500")
    expect(params.get("timezone")).toBe("Asia/Kolkata")
    expect(query).not.toContain("{")
    expect(query).not.toContain("|")
  })

  it("leaves out from and timezone when they are not set", () => {
    const params = new URLSearchParams(
      tailUrl("ds", "error", null, undefined).split("?")[1]
    )
    expect([...params.keys()]).toEqual(["query", "limit"])
  })
})

describe("parseTailEvent", () => {
  it("reads a ready event", () => {
    expect(
      parseTailEvent("ready", '{"datasource_uid":"ds","from":"now-10s"}')
    ).toEqual({ type: "ready", from: "now-10s" })
  })

  it("reads a lines event with its frames, newest time and truncation", () => {
    const frames = [
      {
        ref: "A",
        fields: [
          { name: "TimeNs", type: "string", values: ["1762516800123456789"] },
        ],
      },
    ]
    expect(
      parseTailEvent(
        "lines",
        JSON.stringify({
          frames,
          newest: "2026-11-07T12:00:00.123456789Z",
          truncated: true,
        })
      )
    ).toEqual({
      type: "lines",
      frames,
      newest: "2026-11-07T12:00:00.123456789Z",
      truncated: true,
    })
  })

  it("treats a missing truncated flag as false", () => {
    expect(
      parseTailEvent("lines", '{"frames":[],"newest":"2026-11-07T12:00:00Z"}')
    ).toMatchObject({ type: "lines", truncated: false })
  })

  it("reads an error event", () => {
    expect(
      parseTailEvent("error", '{"code":"query_invalid","error":"bad query"}')
    ).toEqual({ type: "error", code: "query_invalid", error: "bad query" })
  })

  it.each([
    ["lines", "{not json"],
    ["lines", "[]"],
    ["lines", '{"frames":"x","newest":"t"}'],
    ["lines", '{"frames":[]}'],
    ["ready", "{}"],
    ["error", '{"code":"x"}'],
    ["ping", "{}"],
  ])("gives null for a malformed %s event: %s", (name, data) => {
    expect(parseTailEvent(name, data)).toBeNull()
  })
})

describe("tailStartFrom", () => {
  it("starts at the newest line with millisecond precision", () => {
    expect(tailStartFrom(Date.UTC(2026, 10, 7, 12, 0, 0, 123), 0)).toBe(
      "2026-11-07T12:00:00.123Z"
    )
  })

  it("starts 10 seconds back when the tail has no line yet", () => {
    const now = Date.UTC(2026, 10, 7, 12, 0, 30, 0)
    expect(tailStartFrom(null, now)).toBe("2026-11-07T12:00:20.000Z")
  })
})

describe("tailFailureText", () => {
  it("says how many tails a person may hold when the cap is reached", () => {
    expect(tailFailureText(429, "too_many_streams", "too many")).toBe(
      "You have 5 live tails open. Stop one to start another."
    )
  })

  it("keeps the server's text for every other failure", () => {
    expect(tailFailureText(422, "query_invalid", "Not a log query.")).toBe(
      "Not a log query."
    )
    expect(tailFailureText(503, "too_many_streams", "Server is busy.")).toBe(
      "Server is busy."
    )
  })
})
