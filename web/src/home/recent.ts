import { useEffect, useState } from "react"

const recentKeys = {
  dashboard: "slashobserve-recent-dashboards",
  observation: "slashobserve-recent-observations",
} as const

export type RecentKind = keyof typeof recentKeys
export const recentLimit = 20

export type RecentEntry = { id: string; title: string; viewed_at: string }

function isRecent(entry: unknown): entry is RecentEntry {
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

export function parseRecent(raw: string | null): RecentEntry[] {
  if (raw === null) return []
  let parsed: unknown
  try {
    parsed = JSON.parse(raw)
  } catch {
    return []
  }
  if (!Array.isArray(parsed)) return []
  const seen = new Set<string>()
  const out: RecentEntry[] = []
  for (const entry of parsed) {
    if (!isRecent(entry) || seen.has(entry.id)) continue
    seen.add(entry.id)
    out.push({ id: entry.id, title: entry.title, viewed_at: entry.viewed_at })
  }
  return out.slice(0, recentLimit)
}

export function recordRecent(
  list: readonly RecentEntry[],
  entry: { id: string; title: string },
  now: Date
): RecentEntry[] {
  return [
    { id: entry.id, title: entry.title, viewed_at: now.toISOString() },
    ...list.filter((recent) => recent.id !== entry.id),
  ].slice(0, recentLimit)
}

function readRecent(kind: RecentKind): RecentEntry[] {
  try {
    return parseRecent(localStorage.getItem(recentKeys[kind]))
  } catch {
    return []
  }
}

function writeRecent(kind: RecentKind, list: readonly RecentEntry[]) {
  try {
    localStorage.setItem(recentKeys[kind], JSON.stringify(list))
  } catch {
    return
  }
}

export function useRecordRecent(
  kind: RecentKind,
  id: string,
  title: string | undefined
) {
  useEffect(() => {
    if (title === undefined) return
    writeRecent(kind, recordRecent(readRecent(kind), { id, title }, new Date()))
  }, [kind, id, title])
}

export function useRecent(kind: RecentKind): RecentEntry[] {
  const [recent] = useState(() => readRecent(kind))
  return recent
}
