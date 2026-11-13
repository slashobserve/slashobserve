import { describe, expect, test } from "vitest"

import { selectedSectionId, settingsGroups, settingsPanel } from "./sections"

const layout = (role: Parameters<typeof settingsGroups>[0]) =>
  settingsGroups(role).map((group) => ({
    label: group.label,
    ids: group.sections.map((section) => section.id),
  }))

describe("settingsGroups", () => {
  test("groups an admin's sections under You, Workspace and Integrations", () => {
    expect(layout("admin")).toEqual([
      { label: "You", ids: ["appearance", "connected-agents"] },
      {
        label: "Workspace",
        ids: ["server", "sign-in", "outbound-access", "agent-access"],
      },
      { label: "Integrations", ids: ["grafana", "webhooks"] },
    ])
  })

  test("leaves Agent access and Integrations to admins", () => {
    for (const role of ["editor", "commenter", "viewer"] as const) {
      expect(layout(role)).toEqual([
        { label: "You", ids: ["appearance", "connected-agents"] },
        { label: "Workspace", ids: ["server", "sign-in", "outbound-access"] },
      ])
    }
  })

  test("labels the sections", () => {
    expect(
      settingsGroups("admin")
        .flatMap((group) => group.sections)
        .map((section) => section.label)
    ).toEqual([
      "Appearance",
      "Connected agents",
      "Server",
      "Sign-in",
      "Outbound access",
      "Agent access",
      "Grafana connection",
      "Webhooks",
    ])
  })
})

describe("selectedSectionId", () => {
  const admin = settingsGroups("admin")
  const viewer = settingsGroups("viewer")

  test("selects the section the hash names", () => {
    expect(selectedSectionId(admin, "server")).toBe("server")
    expect(selectedSectionId(viewer, "outbound-access")).toBe("outbound-access")
    expect(selectedSectionId(viewer, "connected-agents")).toBe(
      "connected-agents"
    )
    expect(selectedSectionId(viewer, "appearance")).toBe("appearance")
    expect(selectedSectionId(admin, "agent-access")).toBe("agent-access")
    expect(selectedSectionId(admin, "grafana")).toBe("grafana")
    expect(selectedSectionId(admin, "webhooks")).toBe("webhooks")
  })

  test("selects the server without a hash", () => {
    expect(selectedSectionId(admin, "")).toBe("server")
    expect(selectedSectionId(viewer, "")).toBe("server")
  })

  test("sends the old Agent hash to the server", () => {
    expect(selectedSectionId(admin, "agent")).toBe("server")
    expect(selectedSectionId(viewer, "agent")).toBe("server")
  })

  test("falls back to the server for an unknown or differently cased hash", () => {
    expect(selectedSectionId(admin, "keys")).toBe("server")
    expect(selectedSectionId(admin, "Server")).toBe("server")
    expect(selectedSectionId(admin, "Webhooks")).toBe("server")
  })

  test("falls back to the server for a section the role cannot see", () => {
    for (const role of ["editor", "commenter", "viewer"] as const) {
      const groups = settingsGroups(role)
      for (const hash of ["agent-access", "grafana", "webhooks"]) {
        expect(selectedSectionId(groups, hash)).toBe("server")
      }
    }
  })
})

describe("settingsPanel", () => {
  test("opens the Grafana add panel on the Grafana section", () => {
    expect(settingsPanel("grafana", { panel: "grafana-add" })).toEqual({
      kind: "grafana-add",
    })
  })

  test("opens the webhook add panel on the Webhooks section", () => {
    expect(settingsPanel("webhooks", { panel: "webhook-add" })).toEqual({
      kind: "webhook-add",
    })
  })

  test("opens the webhook edit panel with the webhook's id", () => {
    expect(
      settingsPanel("webhooks", { panel: "webhook-edit", webhook: "w1" })
    ).toEqual({ kind: "webhook-edit", webhookId: "w1" })
  })

  test("opens no panel on a section it does not belong to", () => {
    expect(settingsPanel("server", { panel: "grafana-add" })).toBeNull()
    expect(settingsPanel("webhooks", { panel: "grafana-add" })).toBeNull()
    expect(settingsPanel("grafana", { panel: "webhook-add" })).toBeNull()
    expect(
      settingsPanel("grafana", { panel: "webhook-edit", webhook: "w1" })
    ).toBeNull()
  })

  test("opens no edit panel without a webhook id", () => {
    expect(settingsPanel("webhooks", { panel: "webhook-edit" })).toBeNull()
    expect(
      settingsPanel("webhooks", { panel: "webhook-edit", webhook: "" })
    ).toBeNull()
  })

  test("opens no panel for an unknown or missing panel key", () => {
    expect(settingsPanel("webhooks", { panel: "export" })).toBeNull()
    expect(settingsPanel("grafana", { panel: "export" })).toBeNull()
    expect(settingsPanel("webhooks", {})).toBeNull()
  })
})
