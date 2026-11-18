import { Outlet, createFileRoute, redirect } from "@tanstack/react-router"

import { getSession } from "@/lib/auth"
import { SkipLink } from "@/shell/skip-link"

function AuthedLayout() {
  return (
    <>
      <SkipLink />
      <Outlet />
    </>
  )
}

export const Route = createFileRoute("/_authed")({
  beforeLoad: async ({ location }) => {
    const session = await getSession()
    if (!session.authenticated) {
      throw redirect({ to: "/login", search: { return_to: location.href } })
    }
    return { principal: session.principal }
  },
  component: AuthedLayout,
})
