import { describe, expect, test } from "vitest"

import { selectedSectionId, settingsSections } from "./sections"

const ids = (role: Parameters<typeof settingsSections>[0]) =>
  settingsSections(role).map((section) => section.id)

describe("settingsSections", () => {
  test("lists Grafana and Webhooks before Appearance, for admins only", () => {
    expect(ids("admin")).toEqual([
      "agent",
      "server",
      "sign-in",
      "outbound-access",
      "grafana",
      "webhooks",
      "appearance",
    ])
    for (const role of ["editor", "commenter", "viewer"] as const) {
      expect(ids(role)).toEqual([
        "agent",
        "server",
        "sign-in",
        "outbound-access",
        "appearance",
      ])
    }
  })

  test("ends with Appearance for every role", () => {
    for (const role of ["admin", "editor", "commenter", "viewer"] as const) {
      expect(ids(role).at(-1)).toBe("appearance")
    }
  })
})

describe("selectedSectionId", () => {
  const admin = settingsSections("admin")
  const viewer = settingsSections("viewer")

  test("selects the section the hash names", () => {
    expect(selectedSectionId(admin, "server")).toBe("server")
    expect(selectedSectionId(viewer, "outbound-access")).toBe("outbound-access")
    expect(selectedSectionId(admin, "grafana")).toBe("grafana")
    expect(selectedSectionId(admin, "webhooks")).toBe("webhooks")
    expect(selectedSectionId(viewer, "appearance")).toBe("appearance")
  })

  test("selects the agent without a hash", () => {
    expect(selectedSectionId(admin, "")).toBe("agent")
  })

  test("falls back to the agent for an unknown hash", () => {
    expect(selectedSectionId(admin, "keys")).toBe("agent")
    expect(selectedSectionId(admin, "Server")).toBe("agent")
  })

  test("falls back to the agent for Grafana when the role cannot see it", () => {
    expect(selectedSectionId(viewer, "grafana")).toBe("agent")
  })

  test("falls back to the agent for Webhooks when the role cannot see it", () => {
    for (const role of ["editor", "commenter", "viewer"] as const) {
      expect(selectedSectionId(settingsSections(role), "webhooks")).toBe(
        "agent"
      )
    }
  })
})
