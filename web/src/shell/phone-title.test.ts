import { describe, expect, it } from "vitest"

import { headingInPhoneBar, phoneTitle } from "./phone-title"

describe("phoneTitle", () => {
  it("shows the wordmark on Home whatever the crumbs say", () => {
    expect(phoneTitle("/", [])).toEqual({ kind: "wordmark" })
    expect(phoneTitle("/", [{ label: "Ask" }])).toEqual({ kind: "wordmark" })
  })

  it("shows the wordmark on a page with no crumbs", () => {
    expect(phoneTitle("/settings", [])).toEqual({ kind: "wordmark" })
  })

  it("shows the last crumb as the title", () => {
    expect(phoneTitle("/logs", [{ label: "Logs" }])).toEqual({
      kind: "text",
      label: "Logs",
    })
    expect(
      phoneTitle("/o/x", [
        { label: "Observations", to: "/observations" },
        { label: "Observation" },
      ])
    ).toEqual({ kind: "text", label: "Observation" })
  })

  it("shows the page's own title over the crumbs and over Home", () => {
    expect(phoneTitle("/d/abc", [], "Checkout")).toEqual({
      kind: "text",
      label: "Checkout",
    })
    expect(phoneTitle("/logs", [{ label: "Logs" }], "Checkout")).toEqual({
      kind: "text",
      label: "Checkout",
    })
    expect(phoneTitle("/", [], "Checkout")).toEqual({
      kind: "text",
      label: "Checkout",
    })
  })

  it("falls back to the crumbs when the page's title is empty", () => {
    expect(phoneTitle("/logs", [{ label: "Logs" }], "")).toEqual({
      kind: "text",
      label: "Logs",
    })
    expect(phoneTitle("/d/abc", [], "")).toEqual({ kind: "wordmark" })
  })
})

describe("headingInPhoneBar", () => {
  it("matches a heading equal to the last crumb", () => {
    expect(headingInPhoneBar("Logs", [{ label: "Logs" }])).toBe(true)
  })

  it("keeps a heading that names the item rather than the page", () => {
    expect(
      headingInPhoneBar("Checkout", [
        { label: "Dashboards", to: "/dashboards" },
        { label: "Folder" },
      ])
    ).toBe(false)
  })

  it("keeps a heading when the page has no crumbs", () => {
    expect(headingInPhoneBar("Logs", [])).toBe(false)
  })
})
