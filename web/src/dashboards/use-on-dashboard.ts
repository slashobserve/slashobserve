import { useParams } from "@tanstack/react-router"

export function useOnDashboard(dashboardId: string): boolean {
  return useParams({
    strict: false,
    select: (params: { dashboardId?: string }) =>
      params.dashboardId === dashboardId,
  })
}
