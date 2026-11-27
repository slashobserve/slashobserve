import { describe, expect, it } from "vitest"

import {
  composerMode,
  contentChange,
  appliedNote,
  editNote,
  fileOwnedNote,
  mayEditBesidesContent,
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

describe("contentChange", () => {
  it.each([
    [{ edit: true, comment: true }, false, "edit"],
    [{ comment: true }, false, "propose"],
    [{}, false, "none"],
    [{ edit: true, comment: true }, true, "none"],
    [{ comment: true }, true, "none"],
  ] as const)("maps %j, provisioned %s, to %s", (flags, provisioned, want) => {
    expect(contentChange(permissions(flags), provisioned)).toBe(want)
  })
})

describe("mayEditBesidesContent", () => {
  it("stays true on a starter, where deleting, annotating and asking still work", () => {
    expect(mayEditBesidesContent(permissions({ edit: true }), false)).toBe(true)
    expect(mayEditBesidesContent(permissions({}), true)).toBe(true)
    expect(mayEditBesidesContent(permissions({ comment: true }), false)).toBe(
      false
    )
  })
})

describe("editNote", () => {
  it.each([
    ["file", { edit: true }, fileOwnedNote],
    ["file", { comment: true }, null],
    ["file", {}, null],
    ["apply", { edit: true }, appliedNote],
    ["apply", { comment: true }, null],
    ["starter", { edit: true }, null],
    ["workspace", { edit: true }, null],
  ] as const)(
    "on a dashboard managed by %s with permissions %j says %j",
    (managedBy, flags, expected) => {
      expect(
        editNote({ managed_by: managedBy, permissions: permissions(flags) })
      ).toBe(expected)
    }
  )
})

describe("the notes on a dashboard that a file writes", () => {
  it.each([fileOwnedNote, appliedNote])(
    "say what replaces an edit without promising that it will: %s",
    (note) => {
      expect(note).not.toMatch(/the next/i)
      expect(note).toMatch(/replaces edits made here/)
    }
  )
})
