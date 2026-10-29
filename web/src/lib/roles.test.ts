import { describe, expect, test } from "vitest"

import {
  roleAtLeast,
  roleDescriptions,
  roleLabels,
  roles,
  type Role,
} from "./roles"

describe("roles", () => {
  test("lists the four roles highest first", () => {
    expect(roles).toEqual(["admin", "editor", "commenter", "viewer"])
  })

  test("every role has a label and a description", () => {
    expect(roleLabels).toEqual({
      admin: "Admin",
      editor: "Editor",
      commenter: "Commenter",
      viewer: "Viewer",
    })
    for (const role of roles) {
      expect(roleDescriptions[role]).not.toBe("")
    }
  })
})

describe("roleAtLeast", () => {
  const cases: [Role, Role, boolean][] = [
    ["admin", "admin", true],
    ["admin", "editor", true],
    ["admin", "commenter", true],
    ["admin", "viewer", true],
    ["editor", "admin", false],
    ["editor", "editor", true],
    ["editor", "commenter", true],
    ["editor", "viewer", true],
    ["commenter", "admin", false],
    ["commenter", "editor", false],
    ["commenter", "commenter", true],
    ["commenter", "viewer", true],
    ["viewer", "admin", false],
    ["viewer", "editor", false],
    ["viewer", "commenter", false],
    ["viewer", "viewer", true],
  ]

  test.each(cases)("%s against minimum %s is %s", (role, min, expected) => {
    expect(roleAtLeast(role, min)).toBe(expected)
  })
})
