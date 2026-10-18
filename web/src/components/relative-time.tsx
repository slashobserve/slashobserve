import { useSyncExternalStore } from "react"

import { formatDateTime, formatShortRelative } from "@/lib/time"

type Clock = {
  subscribe: (listener: () => void) => () => void
  getSnapshot: () => number
}

const clocks = new Map<number, Clock>()

function createClock(intervalMs: number): Clock {
  const listeners = new Set<() => void>()
  let now = Date.now()
  let timer: ReturnType<typeof setInterval> | undefined
  return {
    subscribe(listener) {
      listeners.add(listener)
      if (listeners.size === 1) {
        now = Date.now()
        timer = setInterval(() => {
          now = Date.now()
          for (const notify of listeners) notify()
        }, intervalMs)
      }
      return () => {
        listeners.delete(listener)
        if (listeners.size === 0) {
          clearInterval(timer)
          timer = undefined
        }
      }
    },
    getSnapshot: () => now,
  }
}

function clockFor(intervalMs: number): Clock {
  let clock = clocks.get(intervalMs)
  if (!clock) {
    clock = createClock(intervalMs)
    clocks.set(intervalMs, clock)
  }
  return clock
}

export function useNow(intervalMs = 30_000): number {
  const { subscribe, getSnapshot } = clockFor(intervalMs)
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot)
}

type Props = { iso: string | null; fallback?: string }

export function RelativeTime({ iso, fallback = "Never" }: Props) {
  const now = useNow()
  if (iso === null) return <>{fallback}</>
  return (
    <time dateTime={iso} title={formatDateTime(iso)}>
      {formatShortRelative(iso, now)}
    </time>
  )
}
