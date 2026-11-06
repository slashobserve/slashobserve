import { describe, expect, it } from "vitest"

import { shift, zoomOut } from "./time-shift"

const now = Date.parse("2026-11-06T12:00:00Z")

describe("zoomOut", () => {
  it.each([
    [
      "doubles an absolute range around its center",
      "2026-11-06T10:00:00Z",
      "2026-11-06T11:00:00Z",
      { from: "2026-11-06T09:30:00.000Z", to: "2026-11-06T11:30:00.000Z" },
    ],
    [
      "turns a relative range into absolute times ending at now",
      "now-1h",
      "now",
      { from: "2026-11-06T10:00:00.000Z", to: "2026-11-06T12:00:00.000Z" },
    ],
    [
      "reads epoch milliseconds",
      String(Date.parse("2026-11-06T08:00:00Z")),
      String(Date.parse("2026-11-06T09:00:00Z")),
      { from: "2026-11-06T07:30:00.000Z", to: "2026-11-06T09:30:00.000Z" },
    ],
    [
      "keeps the doubled length when it would pass now",
      "2026-11-06T11:00:00Z",
      "2026-11-06T11:50:00Z",
      { from: "2026-11-06T10:20:00.000Z", to: "2026-11-06T12:00:00.000Z" },
    ],
  ])("%s", (_name, from, to, expected) => {
    expect(zoomOut(from, to, now)).toEqual(expected)
  })

  it.each([
    ["an unreadable time", "yesterday", "now"],
    ["an empty range", "now", "now"],
    ["a reversed range", "now", "now-1h"],
  ])("answers null for %s", (_name, from, to) => {
    expect(zoomOut(from, to, now)).toBeNull()
  })
})

describe("shift", () => {
  it.each([
    [
      "moves a relative range back by half its length",
      "now-1h",
      "now",
      -1,
      { from: "2026-11-06T10:30:00.000Z", to: "2026-11-06T11:30:00.000Z" },
    ],
    [
      "moves an absolute range forward by half its length",
      "2026-11-06T09:00:00Z",
      "2026-11-06T10:00:00Z",
      1,
      { from: "2026-11-06T09:30:00.000Z", to: "2026-11-06T10:30:00.000Z" },
    ],
    [
      "stops at now when moving forward",
      "2026-11-06T11:00:00Z",
      "2026-11-06T11:50:00Z",
      1,
      { from: "2026-11-06T11:10:00.000Z", to: "2026-11-06T12:00:00.000Z" },
    ],
    [
      "leaves a range that already ends at now where it is",
      "now-6h",
      "now",
      1,
      { from: "2026-11-06T06:00:00.000Z", to: "2026-11-06T12:00:00.000Z" },
    ],
  ] as const)("%s", (_name, from, to, direction, expected) => {
    expect(shift(from, to, now, direction)).toEqual(expected)
  })

  it("rounds a rounded relative range in the given zone", () => {
    expect(shift("now/d", "now/d", now, -1, { zone: "Asia/Kolkata" })).toEqual({
      from: "2026-11-05T06:30:00.000Z",
      to: "2026-11-06T06:30:00.000Z",
    })
  })

  it("answers null for an unreadable range", () => {
    expect(shift("soon", "now", now, 1)).toBeNull()
  })
})
