import { Outlet, createFileRoute } from "@tanstack/react-router"

import { LegacyHeader } from "@/shell/legacy-header"

export const Route = createFileRoute("/_authed/dashboards/$dashboardId")({
  component: DashboardLayout,
})

function DashboardLayout() {
  return (
    <>
      <LegacyHeader />
      <Outlet />
    </>
  )
}
