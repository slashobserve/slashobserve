import type { Chat } from "./chats-queries"

export const noSubjectLabel = "No dashboard or observation"
const unseenDashboardLabel = "A dashboard you cannot see"
const unseenObservationLabel = "An observation you cannot see"
const untitledObservationLabel = "An observation"
const chosenDashboardLabel = "Chosen dashboard"

export function turnsText(turns: number): string {
  return `${turns.toLocaleString()} ${turns === 1 ? "turn" : "turns"}`
}

export function dashboardLabel(
  chat: Pick<Chat, "dashboard_id" | "dashboard_title">
): string | null {
  if (chat.dashboard_id === null) return null
  return chat.dashboard_title ?? unseenDashboardLabel
}

export type Subject = {
  dashboard_id: string | null
  dashboard_title: string | null
  observation_id: string | null
  observation_title?: string | null
}

export function subjectLabel(subject: Subject): string | null {
  if (subject.dashboard_id !== null) {
    return subject.dashboard_title ?? unseenDashboardLabel
  }
  if (subject.observation_id === null) return null
  switch (subject.observation_title) {
    case undefined:
      return untitledObservationLabel
    case null:
      return unseenObservationLabel
    default:
      return subject.observation_title
  }
}

export type DashboardChoice = { id: string; label: string }

export function dashboardChoices(
  chats: readonly Chat[],
  chosen: string | undefined
): DashboardChoice[] {
  const labels = new Map<string, string>()
  for (const chat of chats) {
    const label = dashboardLabel(chat)
    if (chat.dashboard_id !== null && label !== null) {
      labels.set(chat.dashboard_id, label)
    }
  }
  if (chosen !== undefined && !labels.has(chosen)) {
    labels.set(chosen, chosenDashboardLabel)
  }
  return [...labels]
    .map(([id, label]) => ({ id, label }))
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
