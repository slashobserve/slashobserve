import { describe, expect, it } from "vitest"

import type { Folder } from "@/folders/queries"
import { validateSpecSchema } from "@/spec/validate"

import { blankDashboardSpec, blankDashboardTargets } from "./blank-dashboard"

function folder(
  id: string,
  create: boolean,
  kind: "personal" | "general" | "shared" = "shared"
): Folder {
  return {
    id,
    title: id,
    general: kind === "general",
    personal: kind === "personal",
    restricted: false,
    admins_only: false,
    audit: false,
    kind: "open",
    allowed_actions: [],
    dashboard_count: 0,
    grantees: [],
    grantee_count: 0,
    created_by: null,
    permissions: { comment: true, edit: create, manage: create, create },
    level: create ? "admin" : "comment",
  }
}

describe("blankDashboardSpec", () => {
  it("is a valid Spec holding one text panel with the trimmed title", () => {
    const spec = blankDashboardSpec("  Checkout  ")
    expect(validateSpecSchema(spec).valid).toBe(true)
    expect(spec.title).toBe("Checkout")
    expect(spec.panels.map((panel) => panel.type)).toEqual(["text"])
  })
})

describe("blankDashboardTargets", () => {
  it("offers only folders the caller can create in, starting in My Dashboards", () => {
    const targets = blankDashboardTargets([
      folder("g", true, "general"),
      folder("ro", false),
      folder("me", true, "personal"),
    ])
    expect(targets.choices.map((choice) => choice.id)).toEqual(["g", "me"])
    expect(targets.initial).toBe("me")
  })

  it("falls back to General, then the first folder, then none", () => {
    expect(
      blankDashboardTargets([folder("a", true), folder("g", true, "general")])
        .initial
    ).toBe("g")
    expect(blankDashboardTargets([folder("a", true)]).initial).toBe("a")
    expect(blankDashboardTargets([folder("a", false)]).initial).toBeNull()
  })
})
