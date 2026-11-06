import { createFileRoute, redirect } from "@tanstack/react-router"

export const Route = createFileRoute("/_authed/_shell/settings/grafana")({
  beforeLoad: ({ location }) => {
    throw redirect({
      to: "/settings",
      search: location.search,
      hash: "grafana",
      replace: true,
    })
  },
})
