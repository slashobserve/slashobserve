import { describe, expect, it } from "vitest"

import { askOnLabel, askOnOf } from "./ask-on"

describe("askOnOf", () => {
  it("reads the dashboard a finding names", () => {
    expect(
      askOnOf({ ask_on: { dashboard_id: "d1", title: "Checkout" } })
    ).toEqual({ dashboardId: "d1", title: "Checkout" })
  })

  it("is null when the finding names none", () => {
    expect(askOnOf({ ask_on: null })).toBeNull()
    expect(askOnOf({})).toBeNull()
  })

  it("is null for a malformed value", () => {
    expect(askOnOf({ ask_on: { dashboard_id: "", title: "x" } })).toBeNull()
    expect(askOnOf({ ask_on: { dashboard_id: "d1" } })).toBeNull()
    expect(askOnOf({ ask_on: "d1" })).toBeNull()
  })
})

describe("askOnLabel", () => {
  it("names the dashboard", () => {
    expect(askOnLabel("Checkout")).toBe("Ask this on Checkout")
  })

  it("shortens a long title", () => {
    const label = askOnLabel("x".repeat(80))
    expect(label).toBe(`Ask this on ${"x".repeat(37)}...`)
  })
})
