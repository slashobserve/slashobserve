import { useEffect, useState } from "react"

const recentDashboardsKey = "slashobserve-recent-dashboards"
export const recentLimit = 20

export type RecentDashboard = { id: string; title: string; viewed_at: string }

function isRecent(entry: unknown): entry is RecentDashboard {
  if (typeof entry !== "object" || entry === null) return false
  const { id, title, viewed_at } = entry as Record<string, unknown>
  return (
    typeof id === "string" &&
    id !== "" &&
    typeof title === "string" &&
    typeof viewed_at === "string" &&
    !Number.isNaN(Date.parse(viewed_at))
  )
}

export function parseRecent(raw: string | null): RecentDashboard[] {
  if (raw === null) return []
  let parsed: unknown
  try {
    parsed = JSON.parse(raw)
  } catch {
    return []
  }
  if (!Array.isArray(parsed)) return []
  const seen = new Set<string>()
  const out: RecentDashboard[] = []
  for (const entry of parsed) {
    if (!isRecent(entry) || seen.has(entry.id)) continue
    seen.add(entry.id)
    out.push({ id: entry.id, title: entry.title, viewed_at: entry.viewed_at })
  }
  return out.slice(0, recentLimit)
}

export function recordRecent(
  list: readonly RecentDashboard[],
  entry: { id: string; title: string },
  now: Date
): RecentDashboard[] {
  return [
    { id: entry.id, title: entry.title, viewed_at: now.toISOString() },
    ...list.filter((recent) => recent.id !== entry.id),
  ].slice(0, recentLimit)
}

export function recentToShow(
  list: readonly RecentDashboard[],
  starredIds: ReadonlySet<string>,
  limit: number
): RecentDashboard[] {
  return list.filter((recent) => !starredIds.has(recent.id)).slice(0, limit)
}

function readRecent(): RecentDashboard[] {
  try {
    return parseRecent(localStorage.getItem(recentDashboardsKey))
  } catch {
    return []
  }
}

function writeRecent(list: readonly RecentDashboard[]) {
  try {
    localStorage.setItem(recentDashboardsKey, JSON.stringify(list))
  } catch {
    return
  }
}

export function useRecordRecent(
  dashboardId: string,
  title: string | undefined
) {
  useEffect(() => {
    if (title === undefined) return
    writeRecent(
      recordRecent(readRecent(), { id: dashboardId, title }, new Date())
    )
  }, [dashboardId, title])
}

export function useRecentDashboards(): RecentDashboard[] {
  const [recent] = useState(readRecent)
  return recent
}
