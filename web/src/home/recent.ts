import { useEffect, useState } from "react"

import { scopedKey } from "@/lib/scoped-storage"

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

export function readRecent(kind: RecentKind): RecentEntry[] {
  const key = scopedKey(recentKeys[kind])
  if (key === null) return []
  try {
    return parseRecent(localStorage.getItem(key))
  } catch {
    return []
  }
}

function writeRecent(kind: RecentKind, list: readonly RecentEntry[]) {
  const key = scopedKey(recentKeys[kind])
  if (key === null) return
  try {
    localStorage.setItem(key, JSON.stringify(list))
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
