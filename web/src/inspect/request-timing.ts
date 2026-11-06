import { useEffect, useState } from "react"
import { hashKey, useQueryClient, type QueryKey } from "@tanstack/react-query"

export type RequestTiming = {
  startedAt: number | null
  lastMs: number | null
  failed: boolean
}

export const noTiming: RequestTiming = {
  startedAt: null,
  lastMs: null,
  failed: false,
}

export type FetchEvent = { kind: "start" | "success" | "error"; at: number }

export function timingReducer(
  state: RequestTiming,
  event: FetchEvent
): RequestTiming {
  if (event.kind === "start") return { ...state, startedAt: event.at }
  if (state.startedAt === null) return state
  return {
    startedAt: null,
    lastMs: event.at - state.startedAt,
    failed: event.kind === "error",
  }
}

function fetchEventKind(action: { type: string }): FetchEvent["kind"] | null {
  if (action.type === "fetch") return "start"
  if (action.type === "success") return "success"
  if (action.type === "error") return "error"
  return null
}

export function useRequestTiming(
  queryKey: QueryKey | undefined
): RequestTiming {
  const queryClient = useQueryClient()
  const hash = queryKey === undefined ? undefined : hashKey(queryKey)
  const [timing, setTiming] = useState({ hash, value: noTiming })
  if (timing.hash !== hash) setTiming({ hash, value: noTiming })
  useEffect(() => {
    if (hash === undefined) return
    return queryClient.getQueryCache().subscribe((event) => {
      if (event.type !== "updated" || event.query.queryHash !== hash) return
      const kind = fetchEventKind(event.action)
      if (kind === null) return
      const at = performance.now()
      setTiming((current) =>
        current.hash === hash
          ? { hash, value: timingReducer(current.value, { kind, at }) }
          : current
      )
    })
  }, [queryClient, hash])
  return timing.hash === hash ? timing.value : noTiming
}
