import type { PreparedFrame } from "../transforms"

export type AlertState = "firing" | "pending"
type AlertSort =
  "alphaAsc" | "alphaDesc" | "importance" | "timeAsc" | "timeDesc"
export type AlertView = "list" | "stat"

export type Alert = {
  key: string
  name: string
  state: AlertState
  severity: string | null
  labels: [string, string][]
  since: number | null
  activeFor: string | null
}

export type AlertCounts = Record<AlertState, number>

export type AlertModel = {
  reason: string | null
  alerts: Alert[]
  total: number
  counts: AlertCounts
}

export const missingAlertsReason =
  "An alert list needs a query on Prometheus ALERTS."

const defaultLimit = 20
const severityOrder = ["critical", "error", "warning", "info"]
const hiddenLabels = new Set([
  "__name__",
  "alertname",
  "alertstate",
  "severity",
])
const collator = new Intl.Collator()

function limitOf(display: Record<string, unknown>): number {
  const { limit } = display
  return typeof limit === "number" && limit >= 1
    ? Math.floor(limit)
    : defaultLimit
}

function sortOf(display: Record<string, unknown>): AlertSort {
  switch (display.sort) {
    case "alphaDesc":
    case "importance":
    case "timeAsc":
    case "timeDesc":
      return display.sort
    default:
      return "alphaAsc"
  }
}

export function viewOf(display: Record<string, unknown>): AlertView {
  return display.viewMode === "stat" ? "stat" : "list"
}

function labelKey(labels: Record<string, string>): string {
  return Object.keys(labels)
    .sort()
    .map((key) => `${key}=${labels[key]}`)
    .join(",")
}

function lastNumber(frame: PreparedFrame): number | null {
  const field = frame.fields.find((candidate) => candidate.type === "number")
  if (!field) return null
  for (let index = field.values.length - 1; index >= 0; index--) {
    const value = field.values[index]
    if (typeof value === "number" && Number.isFinite(value)) return value
  }
  return null
}

function sinceByAlert(frames: PreparedFrame[]): Map<string, number> {
  const since = new Map<string, number>()
  for (const frame of frames) {
    const labels = frame.labels ?? {}
    if (!labels.alertname || labels.alertstate !== undefined) continue
    const seconds = lastNumber(frame)
    if (seconds !== null) since.set(labelKey(labels), seconds * 1000)
  }
  return since
}

function withoutState(labels: Record<string, string>): Record<string, string> {
  return Object.fromEntries(
    Object.entries(labels).filter(([name]) => name !== "alertstate")
  )
}

function alertsOf(frames: PreparedFrame[], now: number): Alert[] {
  const since = sinceByAlert(frames)
  const seen = new Set<string>()
  const alerts: Alert[] = []
  for (const frame of frames) {
    const labels = frame.labels ?? {}
    const { alertname, alertstate, severity } = labels
    if (!alertname) continue
    if (alertstate !== "firing" && alertstate !== "pending") continue
    const key = labelKey(labels)
    if (seen.has(key)) continue
    seen.add(key)
    const startedAt = since.get(labelKey(withoutState(labels))) ?? null
    alerts.push({
      key,
      name: alertname,
      state: alertstate,
      severity: severity || null,
      labels: Object.entries(labels)
        .filter(([name]) => !hiddenLabels.has(name))
        .sort(([a], [b]) => collator.compare(a, b)),
      since: startedAt,
      activeFor: startedAt === null ? null : activeFor(startedAt, now),
    })
  }
  return alerts
}

function severityRank(severity: string | null): number {
  const rank = severityOrder.indexOf(severity?.toLowerCase() ?? "")
  return rank === -1 ? severityOrder.length : rank
}

function byName(a: Alert, b: Alert): number {
  return collator.compare(a.name, b.name) || collator.compare(a.key, b.key)
}

function byImportance(a: Alert, b: Alert): number {
  const state = Number(a.state === "pending") - Number(b.state === "pending")
  return (
    state || severityRank(a.severity) - severityRank(b.severity) || byName(a, b)
  )
}

function bySince(direction: 1 | -1) {
  return (a: Alert, b: Alert): number => {
    if (a.since === null || b.since === null) {
      return Number(a.since === null) - Number(b.since === null) || byName(a, b)
    }
    return direction * (a.since - b.since) || byName(a, b)
  }
}

function comparatorOf(sort: AlertSort): (a: Alert, b: Alert) => number {
  switch (sort) {
    case "alphaDesc":
      return (a, b) => byName(b, a)
    case "importance":
      return byImportance
    case "timeAsc":
      return bySince(1)
    case "timeDesc":
      return bySince(-1)
    case "alphaAsc":
      return byName
  }
}

export function alertModel(
  frames: PreparedFrame[],
  display: Record<string, unknown>,
  now: number
): AlertModel {
  const counts: AlertCounts = { firing: 0, pending: 0 }
  if (frames.length > 0 && !frames.some((frame) => frame.labels?.alertname)) {
    return { reason: missingAlertsReason, alerts: [], total: 0, counts }
  }
  const alerts = alertsOf(frames, now).sort(comparatorOf(sortOf(display)))
  for (const alert of alerts) counts[alert.state]++
  return {
    reason: null,
    alerts: alerts.slice(0, limitOf(display)),
    total: alerts.length,
    counts,
  }
}

export function activeFor(since: number, now: number): string {
  const seconds = Math.max(0, Math.floor((now - since) / 1000))
  if (seconds < 60) return `for ${seconds}s`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `for ${minutes}m`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `for ${hours}h`
  return `for ${Math.floor(hours / 24)}d`
}
