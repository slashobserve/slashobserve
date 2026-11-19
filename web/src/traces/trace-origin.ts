import type { AnyRouter } from "@tanstack/react-router"

export type TraceOrigin = { label: string; href: string }

type Place = { pathname: string; href: string }

const origins = [
  { prefix: "/logs/", label: "Back to logs" },
  { prefix: "/traces/", label: "Back to traces" },
] as const

export function traceOriginOf(place: Place): TraceOrigin | null {
  const origin = origins.find((o) => place.pathname.startsWith(o.prefix))
  return origin ? { label: origin.label, href: place.href } : null
}

let current: TraceOrigin | null = null

export function currentTraceOrigin(): TraceOrigin | null {
  return current
}

export function nextTraceOrigin(
  kept: TraceOrigin | null,
  from: Place | undefined,
  to: Place
): TraceOrigin | null {
  if (!to.pathname.startsWith("/t/") || from === undefined) return kept
  if (from.pathname.startsWith("/t/")) return kept
  return traceOriginOf(from)
}

export function followTraceOrigin(router: AnyRouter) {
  router.subscribe(
    "onBeforeLoad",
    ({ fromLocation, toLocation, pathChanged }) => {
      if (!pathChanged) return
      current = nextTraceOrigin(current, fromLocation, toLocation)
    }
  )
}
