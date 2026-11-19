import { describe, expect, it } from "vitest"

import { validateEdit, validateNew, type Fields } from "./form"

const now = Date.parse("2026-11-18T10:00:00Z")
const where = { dashboardId: "d1", panelId: "p1" }

function fields(over: Partial<Fields> = {}): Fields {
  return {
    text: "Database failover",
    tags: "incident, db",
    from: "2026-11-18 14:22",
    to: "2026-11-18 14:38",
    scope: "dashboard",
    ...over,
  }
}

describe("validateNew", () => {
  it("builds the request for a range on a dashboard", () => {
    expect(validateNew(fields(), where, "UTC", now)).toEqual({
      ok: true,
      body: {
        dashboard_id: "d1",
        time_from: "2026-11-18T14:22:00.000Z",
        time_to: "2026-11-18T14:38:00.000Z",
        text: "Database failover",
        tags: ["incident", "db"],
      },
    })
  })

  it("leaves the end out for a moment and reads the zone", () => {
    const result = validateNew(
      fields({ to: "", from: "2026-11-18 14:22" }),
      where,
      "Asia/Kolkata",
      now
    )
    expect(result).toMatchObject({
      ok: true,
      body: { time_from: "2026-11-18T08:52:00.000Z" },
    })
    expect(result.ok && "time_to" in result.body).toBe(false)
  })

  it("scopes to a panel or to every dashboard", () => {
    expect(
      validateNew(fields({ scope: "panel" }), where, "UTC", now)
    ).toMatchObject({
      ok: true,
      body: { dashboard_id: "d1", panel_id: "p1" },
    })
    const global = validateNew(fields({ scope: "global" }), where, "UTC", now)
    expect(global).toMatchObject({ ok: true, body: { dashboard_id: null } })
    expect(global.ok && "panel_id" in global.body).toBe(false)
  })

  it("names each field that is wrong", () => {
    const result = validateNew(
      fields({ text: "  ", tags: "Bad Tag!", from: "soon", to: "later" }),
      where,
      "UTC",
      now
    )
    expect(result.ok).toBe(false)
    if (result.ok) return
    expect(Object.keys(result.errors).sort()).toEqual([
      "from",
      "tags",
      "text",
      "to",
    ])
  })

  it("refuses an end before the start and too many tags", () => {
    const reversed = validateNew(
      fields({ to: "2026-11-18 14:00" }),
      where,
      "UTC",
      now
    )
    expect(reversed).toEqual({
      ok: false,
      errors: { to: "The end must not be before the start." },
    })
    const many = validateNew(
      fields({ tags: "a b c d e f g h i j k" }),
      where,
      "UTC",
      now
    )
    expect(many.ok).toBe(false)
  })

  it("refuses a start outside the supported years", () => {
    expect(
      validateNew(fields({ from: "1969-12-31 23:00" }), where, "UTC", now).ok
    ).toBe(false)
    expect(
      validateNew(
        fields({ from: "9999-12-31 23:00", to: "" }),
        where,
        "UTC",
        now
      ).ok
    ).toBe(false)
  })
})

describe("validateEdit", () => {
  it("trims the text and splits the tags", () => {
    expect(
      validateEdit({ text: "  fixed  ", tags: "deploy checkout" })
    ).toEqual({
      ok: true,
      body: { text: "fixed", tags: ["deploy", "checkout"] },
    })
  })

  it("accepts no tags and refuses empty text", () => {
    expect(validateEdit({ text: "x", tags: "" })).toMatchObject({
      ok: true,
      body: { tags: [] },
    })
    expect(validateEdit({ text: " ", tags: "" })).toMatchObject({
      ok: false,
      errors: { text: "Enter what happened." },
    })
  })
})
