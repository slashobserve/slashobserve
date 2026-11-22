import { agentRefPath, type AgentRef } from "@/agents/agent-ref"
import type { StatusTone } from "@/components/status-label"
import {
  chatPath,
  dashboardPath,
  findingPath,
  folderPath,
  observationPath,
  proposalPath,
} from "@/lib/links"

import type { AuditEvent } from "./stream"

export const outcomeTones: Record<AuditEvent["outcome"], StatusTone> = {
  success: "ok",
  denied: "warning",
  failed: "critical",
  unknown: "none",
}

export const outcomeLabels: Record<AuditEvent["outcome"], string> = {
  success: "Success",
  denied: "Denied",
  failed: "Failed",
  unknown: "Unknown",
}

function present(value: string | null | undefined): value is string {
  return value !== null && value !== undefined && value !== ""
}

export function actorText(event: AuditEvent): string {
  if (present(event.actor_name)) return event.actor_name
  return event.actor_kind === "system" ? "System" : "Anonymous"
}

export function targetText(event: AuditEvent): string {
  if (present(event.target_name)) return event.target_name
  if (present(event.target_id)) return event.target_id
  return ""
}

const removalAction = /(delete|remove)$/

function targetGone(event: AuditEvent): boolean {
  return event.outcome === "success" && removalAction.test(event.action)
}

function agentPath(ref: AgentRef): string {
  return `/agents/${encodeURIComponent(agentRefPath(ref))}`
}

export function targetPath(event: AuditEvent): string | null {
  const id = event.target_id
  if (!present(id) || targetGone(event)) return null
  switch (event.target_kind) {
    case "dashboard":
      return dashboardPath(id, event.target_name ?? undefined)
    case "folder":
      return folderPath(id)
    case "observation":
      return observationPath(id)
    case "finding":
      return findingPath(id)
    case "proposal":
      return proposalPath(id)
    case "run":
      return chatPath(id)
    case "service_account":
      return agentPath({ kind: "service_account", id })
    case "grant":
      return agentPath({ kind: "grant", id })
    default:
      return null
  }
}

export function originText(event: AuditEvent): string {
  return present(event.origin_client)
    ? `${event.origin} (${event.origin_client})`
    : event.origin
}

export type EventField = { name: string; label: string; value: string }

function fieldLabel(name: string): string {
  const words = name.replaceAll("_", " ")
  return words.charAt(0).toUpperCase() + words.slice(1)
}

export function eventFields(event: AuditEvent): EventField[] {
  return Object.entries(event).flatMap(([name, value]) => {
    if (name === "data" || value === null || value === undefined) return []
    const text = String(value)
    return text === "" ? [] : [{ name, label: fieldLabel(name), value: text }]
  })
}

export function eventData(event: AuditEvent): string | null {
  const data = event.data as unknown
  if (data === null || data === undefined) return null
  if (typeof data === "object" && Object.keys(data).length === 0) return null
  return JSON.stringify(data, null, 2)
}
