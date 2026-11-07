import { useQuery, useQueryClient } from "@tanstack/react-query"

import { exploreKeys } from "@/api/keys"
import type { WireFrame } from "@/dashboards/queries"

import { exploreRequest } from "./explore"
import { mergeTail, startTail, tailWindow, type TailBuffer } from "./frames"

const tailIntervalMs = 2000
export const tailRows = 1000
const tailWindowMs = 300_000

function oldestKept(now: number): number {
  return now - tailWindowMs
}

export function useLiveTail({
  uid,
  query,
  seed,
  running,
  session,
}: {
  uid: string
  query: string
  seed: WireFrame[]
  running: boolean
  session: number
}) {
  const queryClient = useQueryClient()
  const queryKey = exploreKeys.tail(uid, query, session)
  const tail = useQuery({
    queryKey,
    enabled: running,
    initialData: () => {
      const now = Date.now()
      return startTail(seed, now, oldestKept(now))
    },
    queryFn: async ({ signal }) => {
      const now = Date.now()
      const current =
        queryClient.getQueryData<TailBuffer>(queryKey) ??
        startTail(seed, now, oldestKept(now))
      const window = tailWindow(current.newest, now)
      const data = await exploreRequest(
        uid,
        { query, kind: "instant", ...window, limit: tailRows },
        signal
      )
      return mergeTail(current, data.frames, tailRows, oldestKept(Date.now()))
    },
    refetchInterval: (state) =>
      running && state.state.status !== "error" ? tailIntervalMs : false,
    refetchIntervalInBackground: false,
    refetchOnWindowFocus: false,
    retry: false,
    gcTime: 0,
    structuralSharing: false,
  })
  return {
    buffer: tail.data,
    error: tail.error,
    isFetching: tail.isFetching,
  }
}
