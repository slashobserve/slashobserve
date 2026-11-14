import { roleAtLeast, type Role } from "@/lib/roles"

export type SettingsSectionId =
  | "appearance"
  | "connected-agents"
  | "server"
  | "sign-in"
  | "outbound-access"
  | "agent-access"
  | "grafana"
  | "webhooks"

type SettingsSection = { id: SettingsSectionId; label: string }

export type SettingsGroup = {
  label: string
  sections: readonly SettingsSection[]
}

export type SettingsPanel =
  | { kind: "grafana-add" }
  | { kind: "webhook-add" }
  | { kind: "webhook-edit"; webhookId: string }

const youGroup: SettingsGroup = {
  label: "You",
  sections: [
    { id: "appearance", label: "Appearance" },
    { id: "connected-agents", label: "Connected agents" },
  ],
}

const memberWorkspaceSections: readonly SettingsSection[] = [
  { id: "server", label: "Server" },
  { id: "sign-in", label: "Sign-in" },
  { id: "outbound-access", label: "Outbound access" },
]

const memberGroups: readonly SettingsGroup[] = [
  youGroup,
  { label: "Workspace", sections: memberWorkspaceSections },
]

const adminGroups: readonly SettingsGroup[] = [
  youGroup,
  {
    label: "Workspace",
    sections: [
      ...memberWorkspaceSections,
      { id: "agent-access", label: "Agent access" },
    ],
  },
  {
    label: "Integrations",
    sections: [
      { id: "grafana", label: "Grafana connection" },
      { id: "webhooks", label: "Webhooks" },
    ],
  },
]

export function settingsGroups(role: Role): readonly SettingsGroup[] {
  return roleAtLeast(role, "admin") ? adminGroups : memberGroups
}

export function selectedSectionId(
  groups: readonly SettingsGroup[],
  hash: string
): SettingsSectionId {
  const id = hash === "agent" ? "server" : hash
  return (
    groups
      .flatMap((group) => group.sections)
      .find((section) => section.id === id)?.id ?? "server"
  )
}

export function settingsPanel(
  section: SettingsSectionId,
  search: { panel?: string; webhook?: string }
): SettingsPanel | null {
  if (section === "grafana" && search.panel === "grafana-add") {
    return { kind: "grafana-add" }
  }
  if (section !== "webhooks") return null
  if (search.panel === "webhook-add") return { kind: "webhook-add" }
  if (
    search.panel === "webhook-edit" &&
    search.webhook !== undefined &&
    search.webhook !== ""
  ) {
    return { kind: "webhook-edit", webhookId: search.webhook }
  }
  return null
}
