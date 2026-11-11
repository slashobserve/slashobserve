import { roleAtLeast, type Role } from "@/lib/roles"

export type SettingsSectionId =
  | "agent"
  | "server"
  | "sign-in"
  | "outbound-access"
  | "grafana"
  | "webhooks"
  | "appearance"

export type SettingsSection = { id: SettingsSectionId; label: string }

const agentSection: SettingsSection = { id: "agent", label: "Agent" }

const serverSections: readonly SettingsSection[] = [
  agentSection,
  { id: "server", label: "This server" },
  { id: "sign-in", label: "Sign-in" },
  { id: "outbound-access", label: "Outbound access" },
]

const grafanaSection: SettingsSection = {
  id: "grafana",
  label: "Grafana connection",
}

const webhooksSection: SettingsSection = { id: "webhooks", label: "Webhooks" }

const appearanceSection: SettingsSection = {
  id: "appearance",
  label: "Appearance",
}

const everyoneSections: readonly SettingsSection[] = [
  ...serverSections,
  appearanceSection,
]

const adminSections: readonly SettingsSection[] = [
  ...serverSections,
  grafanaSection,
  webhooksSection,
  appearanceSection,
]

export function settingsSections(role: Role): readonly SettingsSection[] {
  return roleAtLeast(role, "admin") ? adminSections : everyoneSections
}

export function selectedSectionId(
  sections: readonly SettingsSection[],
  hash: string
): SettingsSectionId {
  return sections.find((section) => section.id === hash)?.id ?? agentSection.id
}
