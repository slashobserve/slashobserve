import { useCallback } from "react"
import { useQuery, useQueryClient } from "@tanstack/react-query"

import { asDashboardSpec, latestDashboardQuery } from "@/dashboards/queries"
import { allPanels, panelTitle, type Dashboard } from "@/spec"

export type PanelRef = { id: string; title: string }

export function panelRefs(spec: Dashboard): PanelRef[] {
  return allPanels(spec).map(({ panel }) => ({
    id: panel.id,
    title: panelTitle(panel),
  }))
}

export function useLatestPanels(dashboardId: string): PanelRef[] | undefined {
  const queryClient = useQueryClient()
  const select = useCallback(
    (envelope: { version: { spec: unknown } }): PanelRef[] =>
      panelRefs(asDashboardSpec(envelope.version.spec)),
    []
  )
  const query = useQuery({
    ...latestDashboardQuery(queryClient, dashboardId),
    staleTime: Infinity,
    select,
  })
  return query.data
}
