import { create } from "zustand"

import type { ExploreKind } from "./sources"

const storageKey = "slashobserve-explore-datasource"
const rememberedCap = 20

type Memory = Record<ExploreKind, string[]>

function uidList(value: unknown): string[] {
  if (typeof value === "string") return [value]
  if (!Array.isArray(value)) return []
  return value
    .filter((uid): uid is string => typeof uid === "string")
    .slice(0, rememberedCap)
}

export function parseMemory(raw: string | null): Memory {
  try {
    const parsed: unknown = JSON.parse(raw ?? "")
    if (typeof parsed !== "object" || parsed === null) {
      return { logs: [], traces: [] }
    }
    const stored = parsed as Record<string, unknown>
    return { logs: uidList(stored.logs), traces: uidList(stored.traces) }
  } catch {
    return { logs: [], traces: [] }
  }
}

export function rememberIn(list: readonly string[], uid: string): string[] {
  return [uid, ...list.filter((other) => other !== uid)].slice(0, rememberedCap)
}

function read(): string | null {
  try {
    return localStorage.getItem(storageKey)
  } catch {
    return null
  }
}

function write(memory: Memory) {
  try {
    localStorage.setItem(storageKey, JSON.stringify(memory))
  } catch {
    return
  }
}

type MemoryState = Memory & {
  remember: (kind: ExploreKind, uid: string) => void
}

export const useSourceMemory = create<MemoryState>((set, get) => ({
  ...parseMemory(read()),
  remember: (kind, uid) => {
    const { logs, traces } = get()
    const next = { logs, traces, [kind]: rememberIn(get()[kind], uid) }
    write(next)
    set(next)
  },
}))

export function recentSources(kind: ExploreKind): string[] {
  return useSourceMemory.getState()[kind]
}
