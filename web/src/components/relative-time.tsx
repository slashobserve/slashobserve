import { useEffect, useSyncExternalStore } from "react"

import {
  formatDateTime,
  formatShortRelative,
  isSubMinuteOffset,
  relativeTickMs,
} from "@/lib/time"

const listeners = new Set<() => void>()
let now = Date.now()
let fastCount = 0
let timer: ReturnType<typeof setInterval> | undefined

function schedule() {
  clearInterval(timer)
  timer = undefined
  if (listeners.size === 0) return
  timer = setInterval(() => {
    now = Date.now()
    for (const notify of listeners) notify()
  }, relativeTickMs(fastCount))
}

function subscribe(listener: () => void) {
  listeners.add(listener)
  now = Date.now()
  if (listeners.size === 1) schedule()
  return () => {
    listeners.delete(listener)
    if (listeners.size === 0) schedule()
  }
}

function getSnapshot() {
  return now
}

function requestFastTick() {
  fastCount += 1
  if (fastCount === 1) schedule()
  return () => {
    fastCount -= 1
    if (fastCount === 0) schedule()
  }
}

export function useNow(): number {
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot)
}

type Props = { iso: string | null; fallback?: string }

export function RelativeTime({ iso, fallback = "Never" }: Props) {
  const current = useNow()
  const fast = iso !== null && isSubMinuteOffset(iso, current)
  useEffect(() => (fast ? requestFastTick() : undefined), [fast])
  if (iso === null) return <>{fallback}</>
  return (
    <time dateTime={iso} title={formatDateTime(iso)}>
      {formatShortRelative(iso, current)}
    </time>
  )
}
