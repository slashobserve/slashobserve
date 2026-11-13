import type { components } from "@/api/schema.gen"
import { formatShortRelative } from "@/lib/time"

export type Stats = components["schemas"]["AgentRunStats"]
type ServiceAccountPresence =
  components["schemas"]["AgentsOverview"]["service_accounts"][number]["presence"]
export type Scope =
  components["schemas"]["AgentsOverview"]["observe_agent"]["scope"]

const usd = new Intl.NumberFormat("en-US", {
  style: "currency",
  currency: "USD",
})

function costText(cost: number | null): string {
  if (cost === null) return "Not priced"
  if (cost > 0 && cost < 0.01) return "<$0.01"
  return usd.format(cost)
}

function waitText(seconds: number | null): string {
  if (seconds === null) return "No runs"
  const whole = Math.round(seconds)
  if (whole < 60) return `${whole} s`
  const minutes = Math.round(whole / 60)
  if (minutes < 60) return `${minutes} min`
  return `${Math.round(minutes / 60)} h`
}

export function statCells(
  stats: Stats | null
): { label: string; value: string }[] {
  if (stats === null) return []
  return [
    { label: "Runs", value: String(stats.runs) },
    { label: "Failed", value: String(stats.failed) },
    { label: "Cost", value: costText(stats.cost_usd) },
    { label: "Median wait", value: waitText(stats.median_queue_wait_seconds) },
  ]
}

export function scopeLine(scope: Scope): string {
  return scope === "workspace" ? "Last 7 days, everyone" : "Last 7 days, yours"
}

export type Presence = ServiceAccountPresence

export function presenceLabel(
  presence: Presence,
  lastSeenAt: string | null,
  now = Date.now()
): string {
  if (presence === "working") return "Working"
  if (presence === "listening") return "Listening"
  if (lastSeenAt === null) return "Never seen"
  return `Offline, last seen ${formatShortRelative(lastSeenAt, now).toLowerCase()}`
}
