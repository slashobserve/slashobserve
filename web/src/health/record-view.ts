import { useQueryClient } from "@tanstack/react-query"
import { useEffect, useRef } from "react"

import { api } from "@/api/client"
import { dashboardKeys } from "@/api/keys"

export function useRecordView(dashboardId: string) {
  const queryClient = useQueryClient()
  const recorded = useRef<string | null>(null)
  useEffect(() => {
    if (recorded.current === dashboardId) return
    recorded.current = dashboardId
    api
      .POST("/dashboards/{id}/views", {
        params: { path: { id: dashboardId } },
      })
      .then(() =>
        queryClient.invalidateQueries({
          queryKey: dashboardKeys.views(dashboardId),
        })
      )
      .catch(() => undefined)
  }, [dashboardId, queryClient])
}
