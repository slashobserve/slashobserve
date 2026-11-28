import { roleAtLeast, type Role } from "@/lib/roles"

export type SettingsSectionId =
  | "appearance"
  | "connected-agents"
  | "home-dashboard"
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
    { id: "home-dashboard", label: "Home dashboard" },
    { id: "connected-agents", label: "Connected agents" },
  ],
}

const memberWorkspaceSections: readonly SettingsSection[] = [
  { id: "server", label: "Server" },
  { id: "sign-in", label: "Sign-in" },
]

const outboundAccessSection: SettingsSection = {
  id: "outbound-access",
  label: "Outbound access",
}

const memberGroups: readonly SettingsGroup[] = [
  youGroup,
  { label: "Workspace", sections: memberWorkspaceSections },
]

const editorGroups: readonly SettingsGroup[] = [
  youGroup,
  {
    label: "Workspace",
    sections: [...memberWorkspaceSections, outboundAccessSection],
  },
]

const adminGroups: readonly SettingsGroup[] = [
  youGroup,
  {
    label: "Workspace",
    sections: [
      ...memberWorkspaceSections,
      outboundAccessSection,
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

const auditLogHash = "audit-log"
const mcpSourcesHash = "mcp-sources"

export function isMovedAuditLog(hash: string): boolean {
  return hash === auditLogHash
}

export function isMovedMCPSources(hash: string): boolean {
  return hash === mcpSourcesHash
}

export function settingsGroups(role: Role): readonly SettingsGroup[] {
  if (roleAtLeast(role, "admin")) return adminGroups
  return roleAtLeast(role, "editor") ? editorGroups : memberGroups
}

const renamedSections: Record<string, SettingsSectionId> = {
  agent: "server",
  "grafana-connection": "grafana",
}

export function selectedSectionId(
  groups: readonly SettingsGroup[],
  hash: string
): SettingsSectionId {
  const id = renamedSections[hash] ?? hash
  return (
    groups
      .flatMap((group) => group.sections)
      .find((section) => section.id === id)?.id ?? "server"
  )
}

export type SectionNotice = { hash: string; adminOnly: boolean }

export function sectionNotice(
  groups: readonly SettingsGroup[],
  hash: string
): SectionNotice | null {
  if (hash === "" || hash === "agent") return null
  const id = renamedSections[hash] ?? hash
  const known = (list: readonly SettingsGroup[]) =>
    list.flatMap((group) => group.sections).some((section) => section.id === id)
  if (known(groups)) return null
  return { hash, adminOnly: known(adminGroups) }
}

export function settingsPanel(
  section: SettingsSectionId,
  search: { panel?: string | undefined; webhook?: string | undefined }
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
