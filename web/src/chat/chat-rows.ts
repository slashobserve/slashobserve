import type { Chat } from "./chats-queries"

export type DashboardTitles = ReadonlyMap<string, string>

export const noDashboardLabel = "No dashboard"
const unseenDashboardLabel = "A dashboard you cannot see"
const loadingDashboardLabel = "Dashboard"

export function turnsText(turns: number): string {
  return `${turns.toLocaleString()} ${turns === 1 ? "turn" : "turns"}`
}

export function dashboardLabel(
  dashboardId: string | null,
  titles: DashboardTitles | undefined
): string | null {
  if (dashboardId === null) return null
  if (titles === undefined) return loadingDashboardLabel
  return titles.get(dashboardId) ?? unseenDashboardLabel
}

export type DashboardChoice = { id: string; label: string }

export function dashboardChoices(
  chats: readonly Chat[],
  titles: DashboardTitles | undefined,
  chosen: string | undefined
): DashboardChoice[] {
  const ids = new Set<string>()
  for (const chat of chats) {
    if (chat.dashboard_id !== null) ids.add(chat.dashboard_id)
  }
  if (chosen !== undefined) ids.add(chosen)
  return [...ids]
    .map((id) => ({ id, label: dashboardLabel(id, titles) ?? "" }))
    .toSorted((a, b) => a.label.localeCompare(b.label))
}

const weekdays = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"]
const months = [
  "Jan",
  "Feb",
  "Mar",
  "Apr",
  "May",
  "Jun",
  "Jul",
  "Aug",
  "Sep",
  "Oct",
  "Nov",
  "Dec",
]
const dayMs = 86_400_000
const weekDays = 6

function startOfDay(date: Date): number {
  return new Date(date.getFullYear(), date.getMonth(), date.getDate()).getTime()
}

function clock(date: Date): string {
  const pad = (n: number) => String(n).padStart(2, "0")
  return `${pad(date.getHours())}:${pad(date.getMinutes())}`
}

export function chatWhen(iso: string, now: number): string {
  const at = new Date(iso)
  const days = Math.round((startOfDay(new Date(now)) - startOfDay(at)) / dayMs)
  const day =
    days <= 0
      ? "Today"
      : days === 1
        ? "Yesterday"
        : days <= weekDays
          ? weekdays[at.getDay()]
          : `${months[at.getMonth()]} ${at.getDate()}`
  return `${day} ${clock(at)}`
}

export function chatOptionMeta(chat: Chat, now: number): string {
  return `${chatWhen(chat.last_turn_at, now)} · ${turnsText(chat.turns)}`
}
