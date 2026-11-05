import { useQuery, useQueryClient } from "@tanstack/react-query"

import { exploreKeys } from "@/api/keys"
import type { WireFrame } from "@/dashboards/queries"

import { exploreRequest } from "./explore"
import { emptyTail, mergeTail, tailWindow, type TailBuffer } from "./frames"

const tailIntervalMs = 2000
const tailRows = 1000

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
    initialData: () => mergeTail(emptyTail, seed),
    queryFn: async ({ signal }) => {
      const current =
        queryClient.getQueryData<TailBuffer>(queryKey) ??
        mergeTail(emptyTail, seed)
      const window = tailWindow(current.newest, Date.now())
      const data = await exploreRequest(
        uid,
        { query, kind: "instant", ...window, limit: tailRows },
        signal
      )
      return mergeTail(current, data.frames)
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
