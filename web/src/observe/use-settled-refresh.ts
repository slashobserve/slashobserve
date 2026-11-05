import { useEffect, useRef } from "react"
import { useQueryClient } from "@tanstack/react-query"

import { dashboardKeys } from "@/api/keys"

type Settling = {
  active: boolean
  status: "queued" | "running" | "succeeded" | "failed"
  dashboardId: string | null
}

export function useSettledRefresh({ active, status, dashboardId }: Settling) {
  const queryClient = useQueryClient()
  const wasActive = useRef(active)
  useEffect(() => {
    if (wasActive.current && !active) {
      if (status === "succeeded") {
        void queryClient.invalidateQueries({ queryKey: dashboardKeys.list() })
      }
      if (dashboardId !== null) {
        void queryClient.invalidateQueries({
          queryKey: dashboardKeys.revisions(dashboardId),
        })
        void queryClient.invalidateQueries({
          queryKey: dashboardKeys.latest(dashboardId),
        })
      }
    }
    wasActive.current = active
  }, [active, status, dashboardId, queryClient])
}
