import { describe, expect, it } from "vitest"

import {
  composerMode,
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
    expect(composerMode(permissions(flags))).toBe(expected)
  })
})

describe("readerNote", () => {
  it("has no note for the full composer", () => {
    expect(readerNote("full", "Payments")).toBeNull()
  })

  it("names the folder in the comment-only note", () => {
    expect(readerNote("comment-only", "Payments")).toBe(
      "You can view and comment in Payments. Someone with Edit on this folder can send your comment to the agent or change the panels."
    )
  })

  it("names the folder in the read-only note", () => {
    expect(readerNote("read-only", "Payments")).toBe(
      "You can view dashboards in Payments. Ask an Admin if you need to comment."
    )
  })
})
