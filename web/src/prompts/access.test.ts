import { describe, expect, it } from "vitest"

import { canDriveAgent } from "./access"

describe("canDriveAgent", () => {
  const cases: {
    name: string
    principal: { role?: string }
    allowed: boolean
  }[] = [
    { name: "a principal without a role", principal: {}, allowed: true },
    { name: "an admin", principal: { role: "admin" }, allowed: true },
    { name: "an editor", principal: { role: "editor" }, allowed: true },
    {
      name: "a commenter",
      principal: { role: "commenter" },
      allowed: false,
    },
    { name: "a viewer", principal: { role: "viewer" }, allowed: false },
  ]

  it.each(cases)("decides for $name", ({ principal, allowed }) => {
    expect(canDriveAgent(principal)).toBe(allowed)
  })
})
