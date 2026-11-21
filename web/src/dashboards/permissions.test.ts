import { describe, expect, it } from "vitest"

import {
  composerMode,
  mayInvestigate,
  readerNote,
  type DashboardPermissions,
} from "./permissions"

function permissions(
  overrides: Partial<DashboardPermissions>
): DashboardPermissions {
  return {
    comment: false,
    edit: false,
    manage: false,
    create: false,
    ...overrides,
  }
}

describe("composerMode", () => {
  it.each([
    [{ edit: true, comment: true }, "full"],
    [{ edit: true, comment: false }, "full"],
    [{ comment: true }, "comment-only"],
    [{ comment: true, manage: true, create: true }, "comment-only"],
    [{}, "read-only"],
    [{ manage: true, create: true }, "read-only"],
  ] as const)("maps %j to %s", (flags, expected) => {
    expect(composerMode(permissions(flags), false)).toBe(expected)
  })

  it.each([
    [{ comment: true }, "provisioned"],
    [{ comment: true, edit: true }, "provisioned"],
    [{}, "read-only"],
  ] as const)("maps %j on a starter to %s", (flags, expected) => {
    expect(composerMode(permissions(flags), true)).toBe(expected)
  })
})

describe("readerNote", () => {
  it("has no note for the full composer", () => {
    expect(readerNote("full", "Payments")).toBeNull()
  })

  it("names the folder in the comment-only note", () => {
    expect(readerNote("comment-only", "Payments")).toBe(
      "You can view and comment in Payments. What the agent changes for you reaches the dashboard as a proposal that someone with Edit on this folder merges."
    )
  })

  it("says a starter is kept current by releases and can be duplicated", () => {
    const note = readerNote("provisioned", "Audit")
    expect(note).toContain("kept current by SlashObserve releases")
    expect(note).toContain("Duplicate")
  })

  it("names the folder in the read-only note", () => {
    expect(readerNote("read-only", "Payments")).toBe(
      "You can view dashboards in Payments. To comment, ask the people who manage this folder for Comment access."
    )
  })
})

describe("mayInvestigate", () => {
  it("needs Edit on the dashboard and the Editor role", () => {
    const edit = permissions({ comment: true, edit: true })
    expect(mayInvestigate(edit, "editor")).toBe(true)
    expect(mayInvestigate(edit, "admin")).toBe(true)
    expect(mayInvestigate(edit, "viewer")).toBe(false)
    expect(mayInvestigate(edit, "commenter")).toBe(false)
    expect(mayInvestigate(permissions({ comment: true }), "editor")).toBe(false)
  })
})
