import { describe, expect, it } from "vitest"

import { pickDashboard, targetFor, validateByTitleSearch } from "./by-title"

const dashboard = (id: string, title: string) => ({ id, title })

describe("pickDashboard", () => {
  it("picks the one dashboard whose title is equal", () => {
    expect(
      pickDashboard("Thanos / Query", [
        dashboard("a", "Thanos / Query"),
        dashboard("b", "Thanos / Query Frontend"),
      ])
    ).toBe("a")
  })

  it("prefers the exact title over a different case", () => {
    expect(
      pickDashboard("Thanos / Query", [
        dashboard("a", "thanos / query"),
        dashboard("b", "Thanos / Query"),
      ])
    ).toBe("b")
  })

  it("picks the one dashboard whose title differs only in case", () => {
    expect(
      pickDashboard("thanos / query", [
        dashboard("a", "Thanos / Query"),
        dashboard("b", "Thanos / Query Frontend"),
      ])
    ).toBe("a")
  })

  it("picks nothing when two dashboards share the title", () => {
    expect(
      pickDashboard("Thanos / Query", [
        dashboard("a", "Thanos / Query"),
        dashboard("b", "Thanos / Query"),
      ])
    ).toBeUndefined()
  })

  it("picks nothing when two dashboards differ from the title only in case", () => {
    expect(
      pickDashboard("thanos / query", [
        dashboard("a", "Thanos / Query"),
        dashboard("b", "THANOS / QUERY"),
      ])
    ).toBeUndefined()
  })

  it("picks nothing when no title matches", () => {
    expect(
      pickDashboard("Thanos / Query", [dashboard("a", "Thanos / Receive")])
    ).toBeUndefined()
    expect(pickDashboard("Thanos / Query", [])).toBeUndefined()
  })
})

describe("targetFor", () => {
  it("opens the dashboard when one matches", () => {
    expect(targetFor("A", [dashboard("a", "A")])).toEqual({
      kind: "dashboard",
      id: "a",
    })
  })

  it("filters the list by the title when several or none match", () => {
    const twice = [dashboard("a", "A"), dashboard("b", "A")]
    expect(targetFor("A", twice)).toEqual({ kind: "list", title: "A" })
    expect(targetFor("A", [])).toEqual({ kind: "list", title: "A" })
  })

  it("goes to the plain list without a title", () => {
    expect(targetFor(undefined, [])).toEqual({ kind: "none" })
  })
})

describe("validateByTitleSearch", () => {
  it("keeps the title, the time range and the variables only", () => {
    expect(
      validateByTitleSearch({
        title: "Thanos / Query",
        from: "now-6h",
        to: "now",
        "var-job": ["a", "b"],
        "var-bad name": "x",
        refresh: "10s",
        version: "3",
        panel: "p1",
      })
    ).toEqual({
      title: "Thanos / Query",
      from: "now-6h",
      to: "now",
      "var-job": ["a", "b"],
    })
  })

  it("trims the title and keeps the All selection", () => {
    expect(
      validateByTitleSearch({ title: "  Query  ", "var-job": "$__all" })
    ).toEqual({ title: "Query", "var-job": ["$__all"] })
  })

  it("drops a missing, blank or over-long title", () => {
    expect(validateByTitleSearch({})).toEqual({})
    expect(validateByTitleSearch({ title: "   " })).toEqual({})
    expect(validateByTitleSearch({ title: "x".repeat(201) })).toEqual({})
    expect(validateByTitleSearch({ title: "x".repeat(200) })).toEqual({
      title: "x".repeat(200),
    })
    expect(validateByTitleSearch({ title: 5 })).toEqual({})
  })
})
