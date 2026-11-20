import type { components } from "@/api/schema.gen"

export type ActivityEvent = components["schemas"]["ActivityEvent"]
export type ActivityType = components["schemas"]["ActivityType"]

export const auditKinds = [
  { value: "user.role_changed", label: "Role changed" },
  { value: "user.signed_out_everywhere", label: "Signed out everywhere" },
  { value: "user.invited", label: "Person invited" },
  { value: "user.invite_updated", label: "Invite changed" },
  { value: "user.invite_withdrawn", label: "Invite withdrawn" },
  { value: "user.deactivated", label: "Person deactivated" },
  { value: "user.reactivated", label: "Person reactivated" },
  { value: "user.removed", label: "Person removed" },
  { value: "service_account.created", label: "Service account created" },
  { value: "service_account.updated", label: "Service account changed" },
  { value: "service_account.deleted", label: "Service account deleted" },
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
  { value: "datasource.created", label: "Datasource added" },
  { value: "datasource.updated", label: "Datasource changed" },
  {
    value: "datasource.audience_changed",
    label: "Datasource audience changed",
  },
  { value: "datasource.deleted", label: "Datasource deleted" },
  { value: "public_link.created", label: "Public link created" },
  { value: "public_link.revoked", label: "Public link revoked" },
  { value: "access_request.opened", label: "Comment access requested" },
  { value: "access_request.granted", label: "Comment access granted" },
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

function shown(value: unknown): string {
  return Array.isArray(value) ? String(value.length) : String(value)
}

function change(data: Data, key: string): string | null {
  const value = data[key]
  if (typeof value !== "object" || value === null) return null
  const { from, to } = value as { from?: unknown; to?: unknown }
  return `${key.replaceAll("_", " ")} ${shown(from)} to ${shown(to)}`
}

function quoted(kind: string, name: string): string {
  return name === "" ? `a ${kind}` : `the ${kind} "${name}"`
}

function changes(data: Data, keys: readonly string[]): string {
  const source = (data.changes as Data | undefined) ?? data
  return keys
    .map((key) => change(source, key))
    .filter((part) => part !== null)
    .join(", ")
}

export function describeAuditEvent(event: ActivityEvent): string {
  const data = event.data as Data
  const name = str(data, "name")
  switch (event.type) {
    case "user.role_changed":
      if (str(data, "source") !== "")
        return `The sign-in policy changed the role of ${name} from ${str(data, "from_role")} to ${str(data, "to_role")} (${str(data, "source")})`
      return data.manual === false
        ? `Cleared the role override of ${name}; the role is now ${str(data, "to_role")} (was ${str(data, "from_role")})`
        : `Changed the role of ${name} from ${str(data, "from_role")} to ${str(data, "to_role")}`
    case "user.signed_out_everywhere":
      return `Signed ${name} out everywhere (${num(data, "sessions")} sessions, ${num(data, "grants")} connected agents)`
    case "user.invited":
      return `Invited ${str(data, "email")} as ${str(data, "role")}`
    case "user.invite_updated":
      return `Changed the invite of ${str(data, "email")}: ${changes(data, ["role"])}`
    case "user.invite_withdrawn":
      return `Withdrew the invite of ${str(data, "email")}`
    case "user.deactivated":
      return `Deactivated ${name} (${num(data, "sessions")} sessions, ${num(data, "grants")} connected agents)`
    case "user.reactivated":
      return `Reactivated ${name}`
    case "user.removed":
      return `Removed ${name}`
    case "service_account.created":
      return `Created ${quoted("service account", name)} as ${str(data, "role")}`
    case "service_account.updated":
      return `Changed ${quoted("service account", name)}: ${changes(data, ["role", "takes_runs"])}`
    case "service_account.deleted":
      return `Deleted ${quoted("service account", name)}`
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
    case "datasource.created":
      return `Added ${quoted("datasource", name)} at ${str(data, "host")}`
    case "datasource.updated": {
      const parts = [
        changes(data, ["name", "host"]),
        data.address_changed === true ? "address changed" : "",
        data.settings_changed === true ? "settings changed" : "",
        data.credentials_changed === true ? "credentials replaced" : "",
      ].filter((part) => part !== "")
      return `Changed ${quoted("datasource", name)}: ${parts.join(", ")}`
    }
    case "datasource.audience_changed":
      return `Changed the audience of ${quoted("datasource", name)} from ${str(data, "from")} to ${str(data, "to")}`
    case "datasource.deleted":
      return `Deleted ${quoted("datasource", name)}`
    case "public_link.created":
      return `Created a public link to ${linkTarget(data)}`
    case "public_link.revoked":
      return `Revoked a public link to ${linkTarget(data)}`
    case "access_request.opened":
      return `Asked for comment access to ${accessTarget(data)}`
    case "access_request.granted":
      return `Granted comment access to ${accessTarget(data)} asked for by ${requesterName(data)}`
    default:
      return event.type
  }
}

function accessTarget(data: Data): string {
  const title = str(data, "title")
  const object = data.object as { kind?: unknown } | undefined
  const observation = object?.kind === "observation"
  if (title === "") return observation ? "an observation" : "a dashboard"
  return `the ${observation ? "observation" : "dashboard"} "${title}"`
}

function requesterName(data: Data): string {
  const requester = data.requested_by as { name?: unknown } | undefined
  return typeof requester?.name === "string" && requester.name !== ""
    ? requester.name
    : "someone"
}

function linkTarget(data: Data): string {
  const title = str(data, "title")
  const object = data.object as { kind?: unknown } | undefined
  const kind = object?.kind === "observation" ? "RCA" : "dashboard"
  return title === "" ? `a ${kind}` : `the ${kind} "${title}"`
}
