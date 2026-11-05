import type { components } from "@/api/schema.gen"
import type { StatusTone } from "@/components/status-label"
import { isActive } from "@/revisions/queries"

export type RepairItem = components["schemas"]["RepairItem"]
type RepairsQueued = components["schemas"]["RepairsQueued"]

export const maxActiveRepairs = 20

export type RepairState = {
  tone: StatusTone | null
  label: string | null
  canRepair: boolean
  reason: string | null
}

const labels: Record<
  NonNullable<RepairItem["repair"]>["status"],
  { tone: StatusTone; label: string }
> = {
  queued: { tone: "running", label: "Repair queued" },
  running: { tone: "running", label: "Repairing" },
  succeeded: { tone: "ok", label: "Repaired, still failing" },
  failed: { tone: "critical", label: "Repair failed" },
}

function blockedReason(
  item: RepairItem,
  agentAvailable: boolean
): string | null {
  if (!agentAvailable) return "No agent is configured"
  if (item.repair && isActive(item.repair.status)) {
    return "A repair is already queued or running"
  }
  if (item.unverified_panels + item.unverified_variables === 0) {
    return "Placeholders are replaced by hand; the agent does not edit them"
  }
  return null
}

export function repairState(
  item: RepairItem,
  agentAvailable: boolean
): RepairState {
  const shown = item.repair ? labels[item.repair.status] : null
  const reason = blockedReason(item, agentAvailable)
  return {
    tone: shown?.tone ?? null,
    label: shown?.label ?? null,
    canRepair: reason === null,
    reason,
  }
}

export function anyActive(items: RepairItem[]): boolean {
  return items.some(
    (item) => item.repair !== null && isActive(item.repair.status)
  )
}

function count(n: number, one: string, many: string): string {
  return `${n} ${n === 1 ? one : many}`
}

export function unverifiedText(item: RepairItem): string {
  const parts = []
  if (item.unverified_panels > 0) {
    parts.push(count(item.unverified_panels, "panel", "panels"))
  }
  if (item.unverified_variables > 0) {
    parts.push(count(item.unverified_variables, "variable", "variables"))
  }
  return parts.length === 0 ? "None" : parts.join(", ")
}

export function outcomeLines(outcome: RepairsQueued): string[] {
  const full = outcome.skipped.filter((s) => s.reason === "queue_full").length
  const other = outcome.skipped.length - full
  const lines = [
    outcome.queued.length === 0
      ? "No repairs queued"
      : `Queued ${count(outcome.queued.length, "repair", "repairs")}`,
  ]
  if (other > 0) {
    lines.push(
      `Skipped ${other}: already queued or running, or nothing the agent can repair`
    )
  }
  if (full > 0) {
    lines.push(
      `${full} not queued yet: you can have ${maxActiveRepairs} repairs queued or running. Run Repair all again when these finish.`
    )
  }
  return lines
}
