import { validateDashboardSearch } from "@/dashboards/search"

export type DraftLocation = {
  routeId: string
  params: Record<string, string | undefined>
  search: unknown
}

export function leavesDraft(
  current: DraftLocation,
  next: DraftLocation
): boolean {
  if (
    current.routeId !== next.routeId ||
    current.params.dashboardId !== next.params.dashboardId
  ) {
    return true
  }
  const before = validateDashboardSearch(
    current.search as Record<string, unknown>
  )
  const after = validateDashboardSearch(next.search as Record<string, unknown>)
  return (
    before.panel !== after.panel ||
    after.edit === undefined ||
    before.version !== after.version
  )
}
