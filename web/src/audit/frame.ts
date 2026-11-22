import type { AuditView } from "./search"
import type { AuditEvent } from "./stream"

export type Applied = Pick<AuditView, "q" | "from" | "to">

export type Frame = {
  applied: Applied
  seen: string
  anchor: number
  live: readonly AuditEvent[]
}

export function appliedOf(view: AuditView): Applied {
  return { q: view.q, from: view.from, to: view.to }
}

export function appliedKey(applied: Applied): string {
  return JSON.stringify([applied.q, applied.from, applied.to])
}

export function frameOf(applied: Applied, seen: string, anchor: number): Frame {
  return { applied, seen, anchor, live: [] }
}

export function syncFrame(frame: Frame, view: AuditView, now: number): Frame {
  const seen = appliedKey(appliedOf(view))
  if (frame.seen === seen) return frame
  if (appliedKey(frame.applied) === seen) return { ...frame, seen }
  return frameOf(appliedOf(view), seen, now)
}

export function needsCheck(applied: Applied, next: AuditView): boolean {
  return (
    next.from === applied.from &&
    next.to === applied.to &&
    next.q !== applied.q &&
    next.q.trim() !== ""
  )
}
