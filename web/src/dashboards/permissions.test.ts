import { describe, expect, it } from "vitest"

import type { Role } from "@/lib/roles"

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

const roleCases: [Role, string][] = [
  ["admin", "Admin"],
  ["editor", "Editor"],
  ["commenter", "Commenter"],
  ["viewer", "Viewer"],
]

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
    expect(readerNote("full", "editor")).toBeNull()
  })

  it.each(roleCases)(
    "names the %s role in the comment-only note",
    (role, label) => {
      expect(readerNote("comment-only", role)).toBe(
        `You can view and comment as a ${label}. An Editor can send your comment to the agent or change the panels.`
      )
    }
  )

  it.each(roleCases)(
    "names the %s role in the read-only note",
    (role, label) => {
      expect(readerNote("read-only", role)).toBe(
        `You can view this dashboard as a ${label}. Ask an Admin if you need to comment.`
      )
    }
  )
})
