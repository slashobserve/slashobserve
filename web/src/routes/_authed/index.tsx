import { createFileRoute } from "@tanstack/react-router"

import { DashboardList } from "@/dashboards/dashboard-list"

export const Route = createFileRoute("/_authed/")({ component: DashboardList })
