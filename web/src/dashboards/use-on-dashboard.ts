import { useMatches } from "@tanstack/react-router"

const dashboardViewPath = "/d/$dashboardId/{-$slug}"

export function useOnDashboard(dashboardId: string): boolean {
  return useMatches({
    select: (matches) =>
      matches.some(
        (match) =>
          match.fullPath === dashboardViewPath &&
          (match.params as { dashboardId?: string }).dashboardId === dashboardId
      ),
  })
}
