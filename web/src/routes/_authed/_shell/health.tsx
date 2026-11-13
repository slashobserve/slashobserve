import { createFileRoute, redirect } from "@tanstack/react-router"

import { healthRedirectSearch } from "@/health/health-labels"

export const Route = createFileRoute("/_authed/_shell/health")({
  validateSearch: healthRedirectSearch,
  beforeLoad: ({ search }) => {
    throw redirect({ to: "/dashboards", search, replace: true })
  },
})
