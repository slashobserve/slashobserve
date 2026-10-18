import { useCallback } from "react"
import { useQuery, useQueryClient } from "@tanstack/react-query"

import { asDashboardSpec, latestDashboardQuery } from "@/dashboards/queries"

export type PanelRef = { id: string; title: string }

export function useLatestPanels(dashboardId: string): PanelRef[] | undefined {
  const queryClient = useQueryClient()
  const select = useCallback(
    (envelope: { version: { spec: unknown } }): PanelRef[] =>
      asDashboardSpec(envelope.version.spec).panels.map((panel) => ({
        id: panel.id,
        title: panel.title,
      })),
    []
  )
  const query = useQuery({
    ...latestDashboardQuery(queryClient, dashboardId),
    staleTime: Infinity,
    select,
  })
  return query.data
}
