import { describe, expect, it } from "vitest"

import type { Principal } from "@/lib/auth"
import type { Role } from "@/lib/roles"

import { canSavePrompts } from "./access"

function principalWith(role: Role): Principal {
  return { kind: "user", id: "u1", name: "Ada", role }
}

describe("canSavePrompts", () => {
  const cases = [
    { name: "an admin", role: "admin" as const, allowed: true },
    { name: "an editor", role: "editor" as const, allowed: true },
    { name: "a commenter", role: "commenter" as const, allowed: false },
    { name: "a viewer", role: "viewer" as const, allowed: false },
  ]

  it.each(cases)("decides for $name", ({ role, allowed }) => {
    expect(canSavePrompts(principalWith(role))).toBe(allowed)
  })
})
