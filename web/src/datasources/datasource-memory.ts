import { create } from "zustand"

import { exploreKinds, type ExploreKind } from "./sources"

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

function emptyMemory(): Memory {
  return { logs: [], traces: [], metrics: [] }
}

export function parseMemory(raw: string | null): Memory {
  try {
    const parsed: unknown = JSON.parse(raw ?? "")
    if (typeof parsed !== "object" || parsed === null) return emptyMemory()
    const stored = parsed as Record<string, unknown>
    return {
      ...emptyMemory(),
      ...Object.fromEntries(
        exploreKinds.map((kind) => [kind, uidList(stored[kind])])
      ),
    }
  } catch {
    return emptyMemory()
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
  atLoad: Memory
  remember: (kind: ExploreKind, uid: string) => void
}

export const useSourceMemory = create<MemoryState>((set, get) => {
  const stored = parseMemory(read())
  return {
    ...stored,
    atLoad: stored,
    remember: (kind, uid) => {
      const { logs, traces, metrics } = get()
      const next = {
        logs,
        traces,
        metrics,
        [kind]: rememberIn(get()[kind], uid),
      }
      write(next)
      set(next)
    },
  }
})

export function recentSources(kind: ExploreKind): string[] {
  return useSourceMemory.getState()[kind]
}
