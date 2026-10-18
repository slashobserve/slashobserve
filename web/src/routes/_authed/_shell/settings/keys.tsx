import { createFileRoute, redirect } from "@tanstack/react-router"

export const Route = createFileRoute("/_authed/_shell/settings/keys")({
  beforeLoad: () => {
    throw redirect({ to: "/keys", replace: true })
  },
})
