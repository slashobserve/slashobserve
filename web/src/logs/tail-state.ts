import { formatDate } from "@/lib/date-format"

export const arrivalMarkMs = 1500

export type TailState = {
  paused: boolean
  shownKeys: ReadonlySet<string>
  arrivedAt: ReadonlyMap<string, number>
  pendingKeys: ReadonlySet<string>
  pendingCount: number
}

export function tailStarted(keys: readonly string[]): TailState {
  return {
    paused: false,
    shownKeys: new Set(keys),
    arrivedAt: new Map(),
    pendingKeys: new Set(),
    pendingCount: 0,
  }
}

export function tailReceived(
  state: TailState,
  keys: readonly string[],
  now: number,
  kept?: ReadonlySet<string>
): TailState {
  const fresh = [...new Set(keys)].filter(
    (key) => !state.shownKeys.has(key) && !state.pendingKeys.has(key)
  )
  const trimmed = (keys: Iterable<string>) =>
    kept === undefined
      ? new Set(keys)
      : new Set([...keys].filter((key) => kept.has(key)))
  if (state.paused) {
    if (fresh.length === 0) return state
    const pendingKeys = new Set([...state.pendingKeys, ...fresh])
    return {
      ...state,
      shownKeys: trimmed(state.shownKeys),
      pendingKeys,
      pendingCount: pendingKeys.size,
    }
  }
  const arrivedAt = new Map(state.arrivedAt)
  for (const key of fresh) arrivedAt.set(key, now)
  return {
    ...state,
    shownKeys: trimmed([...state.shownKeys, ...fresh]),
    arrivedAt,
  }
}

export function tailPaused(state: TailState): TailState {
  return state.paused ? state : { ...state, paused: true }
}

export function tailResumed(state: TailState, now: number): TailState {
  const arrivedAt = new Map(state.arrivedAt)
  for (const key of state.pendingKeys) arrivedAt.set(key, now)
  return {
    ...state,
    paused: false,
    shownKeys: new Set([...state.shownKeys, ...state.pendingKeys]),
    arrivedAt,
    pendingKeys: new Set(),
    pendingCount: 0,
  }
}

export function tailExpired(state: TailState, now: number): TailState {
  const arrivedAt = new Map(
    [...state.arrivedAt].filter(([, at]) => now - at < arrivalMarkMs)
  )
  return arrivedAt.size === state.arrivedAt.size
    ? state
    : { ...state, arrivedAt }
}

export function recentlyArrived(state: TailState, key: string): boolean {
  return state.arrivedAt.has(key)
}

export function tailVolume(
  times: readonly number[],
  now: number,
  windowMs = 300_000,
  stepMs = 10_000
): { start: number; count: number }[] {
  const buckets = Math.ceil(windowMs / stepMs)
  const origin = now - windowMs
  const out = Array.from({ length: buckets }, (_, index) => ({
    start: origin + index * stepMs,
    count: 0,
  }))
  for (const time of times) {
    if (!Number.isFinite(time) || time < origin || time > now) continue
    out[Math.min(buckets - 1, Math.floor((time - origin) / stepMs))].count++
  }
  return out
}

export function tailStatusText(
  startedAt: number,
  received: number,
  cap: number,
  noun: string,
  streaming = true
): string {
  const since = formatDate(startedAt, "HH:mm:ss")
  const count = `${received.toLocaleString("en")} ${noun} received`
  const newest = cap.toLocaleString("en")
  return streaming
    ? `Streaming since ${since}, ${count}, keeping the newest ${newest}`
    : `Stopped. Streamed since ${since}, ${count}, kept the newest ${newest}`
}
