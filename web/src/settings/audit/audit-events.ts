import type { components } from "@/api/schema.gen"

export type ActivityEvent = components["schemas"]["ActivityEvent"]
export type ActivityType = components["schemas"]["ActivityType"]

export const auditKinds = [
  { value: "user.role_changed", label: "Role changed" },
  { value: "user.signed_out_everywhere", label: "Signed out everywhere" },
  { value: "service_account.created", label: "Service account created" },
  { value: "service_account.updated", label: "Service account changed" },
  { value: "service_account.key_created", label: "Key created" },
  { value: "service_account.key_revoked", label: "Key revoked" },
  { value: "webhook.created", label: "Webhook created" },
  { value: "webhook.updated", label: "Webhook changed" },
  { value: "webhook.secret_rotated", label: "Webhook secret rotated" },
  { value: "webhook.deleted", label: "Webhook deleted" },
  { value: "mcp_client.connected", label: "Agent connected" },
  { value: "mcp_client.disconnected", label: "Agent disconnected" },
  { value: "outbound_domain.added", label: "Outbound domain allowed" },
  { value: "outbound_domain.removed", label: "Outbound domain removed" },
  { value: "grafana_connection.created", label: "Grafana connected" },
  { value: "grafana_connection.updated", label: "Grafana connection changed" },
  { value: "grafana_connection.deleted", label: "Grafana connection removed" },
  { value: "public_link.created", label: "Public link created" },
  { value: "public_link.revoked", label: "Public link revoked" },
] as const satisfies readonly { value: ActivityType; label: string }[]

export type AuditRange = "hour" | "day" | "week" | "month" | "all"

export const auditRanges: { value: AuditRange; label: string }[] = [
  { value: "hour", label: "Last hour" },
  { value: "day", label: "Last 24 hours" },
  { value: "week", label: "Last 7 days" },
  { value: "month", label: "Last 30 days" },
  { value: "all", label: "All time" },
]

const rangeMs: Record<Exclude<AuditRange, "all">, number> = {
  hour: 3_600_000,
  day: 86_400_000,
  week: 7 * 86_400_000,
  month: 30 * 86_400_000,
}

export function rangeSince(range: AuditRange, now: number): string | undefined {
  if (range === "all") return undefined
  return new Date(now - rangeMs[range]).toISOString()
}

type Data = Record<string, unknown>

function str(data: Data, key: string): string {
  const value = data[key]
  return typeof value === "string" ? value : ""
}

function num(data: Data, key: string): number {
  const value = data[key]
  return typeof value === "number" ? value : 0
}

function change(data: Data, key: string): string | null {
  const value = data[key]
  if (typeof value !== "object" || value === null) return null
  const { from, to } = value as { from?: unknown; to?: unknown }
  return `${key.replaceAll("_", " ")} ${String(from)} to ${String(to)}`
}

function quoted(kind: string, name: string): string {
  return name === "" ? `a ${kind}` : `the ${kind} "${name}"`
}

function changes(data: Data, keys: readonly string[]): string {
  return keys
    .map((key) => change(data, key))
    .filter((part) => part !== null)
    .join(", ")
}

export function describeAuditEvent(event: ActivityEvent): string {
  const data = event.data as Data
  const name = str(data, "name")
  switch (event.type) {
    case "user.role_changed":
      return data.manual === false
        ? `Cleared the role override of ${name}; the role is now ${str(data, "to_role")} (was ${str(data, "from_role")})`
        : `Changed the role of ${name} from ${str(data, "from_role")} to ${str(data, "to_role")}`
    case "user.signed_out_everywhere":
      return `Signed ${name} out everywhere (${num(data, "sessions")} sessions, ${num(data, "grants")} connected agents)`
    case "service_account.created":
      return `Created ${quoted("service account", name)} as ${str(data, "role")}`
    case "service_account.updated":
      return `Changed ${quoted("service account", name)}: ${changes(data, ["role", "takes_runs"])}`
    case "service_account.key_created":
      return `Created the key "${str(data, "key_name")}" for ${quoted("service account", name)}`
    case "service_account.key_revoked":
      return `Revoked the key "${str(data, "key_name")}" of ${quoted("service account", name)}`
    case "webhook.created":
      return `Created ${quoted("webhook", name)} to ${str(data, "host")}`
    case "webhook.updated": {
      const parts = changes(data, ["name", "enabled", "event_types", "host"])
      const path = data.url_changed === true ? "address path" : ""
      return `Changed ${quoted("webhook", name)}: ${[parts, path].filter((part) => part !== "").join(", ")}`
    }
    case "webhook.secret_rotated":
      return `Rotated the secret of ${quoted("webhook", name)}`
    case "webhook.deleted":
      return `Deleted ${quoted("webhook", name)}`
    case "mcp_client.connected":
      return `Connected ${quoted("agent", str(data, "client"))} for ${str(data, "person_name")}`
    case "mcp_client.disconnected":
      return `Disconnected ${quoted("agent", str(data, "client"))} of ${str(data, "person_name")} (${str(data, "reason")})`
    case "outbound_domain.added":
      return `Allowed the outbound domain ${str(data, "domain")}`
    case "outbound_domain.removed":
      return `Removed the outbound domain ${str(data, "domain")}`
    case "grafana_connection.created":
      return `Connected Grafana ${str(data, "host")} as "${name}"`
    case "grafana_connection.updated": {
      const parts = changes(data, ["name", "host"])
      const token = data.token_changed === true ? "token replaced" : ""
      return `Changed ${quoted("Grafana connection", name)}: ${[parts, token].filter((part) => part !== "").join(", ")}`
    }
    case "grafana_connection.deleted":
      return `Removed ${quoted("Grafana connection", name)}`
    case "public_link.created":
      return `Created a public link to ${linkTarget(data)}`
    case "public_link.revoked":
      return `Revoked a public link to ${linkTarget(data)}`
    default:
      return event.type
  }
}

function linkTarget(data: Data): string {
  const title = str(data, "title")
  const object = data.object as { kind?: unknown } | undefined
  const kind = object?.kind === "observation" ? "RCA" : "dashboard"
  return title === "" ? `a ${kind}` : `the ${kind} "${title}"`
}
