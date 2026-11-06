import { describe, expect, it } from "vitest"

import type { Variable } from "@/spec"

import {
  expiryFor,
  frozenSelections,
  linkStatus,
  type ExpiryChoice,
} from "./public-links"

const now = new Date(2026, 10, 6, 12, 0, 0).getTime()

describe("expiryFor", () => {
  it("answers null for no expiry", () => {
    expect(expiryFor("none", "", now)).toEqual({ ok: true, expiresAt: null })
  })

  it.each<[ExpiryChoice, string]>([
    ["1d", new Date(2026, 10, 7, 12, 0, 0).toISOString()],
    ["7d", new Date(2026, 10, 13, 12, 0, 0).toISOString()],
    ["30d", new Date(2026, 11, 6, 12, 0, 0).toISOString()],
  ])("adds the days of %s to now", (choice, expected) => {
    expect(expiryFor(choice, "", now)).toEqual({
      ok: true,
      expiresAt: expected,
    })
  })

  it("ends a chosen date at the next local midnight", () => {
    expect(expiryFor("date", "2026-11-30", now)).toEqual({
      ok: true,
      expiresAt: new Date(2026, 11, 1).toISOString(),
    })
  })

  it("accepts today", () => {
    expect(expiryFor("date", "2026-11-06", now)).toEqual({
      ok: true,
      expiresAt: new Date(2026, 10, 7).toISOString(),
    })
  })

  it.each([
    ["", "Choose a date."],
    ["30/11/2026", "Choose a date."],
    ["2026-02-30", "Choose a date."],
    ["2026-13-01", "Choose a date."],
    ["2026-11-05", "Choose today or a later date."],
  ])("refuses %j", (date, problem) => {
    expect(expiryFor("date", date, now)).toEqual({ ok: false, problem })
  })
})

describe("linkStatus", () => {
  it.each([
    [{ expired: true, usable: false }, "Expired"],
    [
      { expired: false, usable: false },
      "Not usable: uses the viewer's identity",
    ],
    [{ expired: false, usable: true }, null],
  ])("%o reads %j", (link, expected) => {
    expect(linkStatus(link)).toBe(expected)
  })
})

describe("frozenSelections", () => {
  const variables: Variable[] = [
    { name: "env", type: "custom", options: ["prod", "dev"] },
    { name: "job", type: "query", datasource: "prom", query: "" },
  ]

  it("keeps only the declared variables with a selection", () => {
    expect(
      frozenSelections(variables, {
        env: ["dev"],
        job: [],
        stray: ["x"],
      })
    ).toEqual({ env: ["dev"] })
  })

  it("answers no selections for a dashboard without variables", () => {
    expect(frozenSelections(undefined, { env: ["dev"] })).toEqual({})
  })
})
