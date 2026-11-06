import { useEffect, useRef } from "react"

import { api } from "@/api/client"

export function useRecordView(dashboardId: string) {
  const recorded = useRef<string | null>(null)
  useEffect(() => {
    if (recorded.current === dashboardId) return
    recorded.current = dashboardId
    api
      .POST("/dashboards/{id}/views", {
        params: { path: { id: dashboardId } },
      })
      .catch(() => undefined)
  }, [dashboardId])
}
