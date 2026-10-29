import { describe, expect, it } from "vitest"

import { canDriveAgent } from "./access"

describe("canDriveAgent", () => {
  const cases = [
    { name: "an admin", principal: { role: "admin" as const }, allowed: true },
    {
      name: "an editor",
      principal: { role: "editor" as const },
      allowed: true,
    },
    {
      name: "a commenter",
      principal: { role: "commenter" as const },
      allowed: false,
    },
    {
      name: "a viewer",
      principal: { role: "viewer" as const },
      allowed: false,
    },
    { name: "a principal without a role", principal: {}, allowed: false },
  ]

  it.each(cases)("decides for $name", ({ principal, allowed }) => {
    expect(canDriveAgent(principal)).toBe(allowed)
  })
})
