import { useState } from "react"
import {
  Link,
  Outlet,
  createFileRoute,
  redirect,
  useNavigate,
} from "@tanstack/react-router"

import { ThemeToggle } from "@/components/theme-toggle"
import { Button } from "@/components/ui/button"
import { getSession, invalidateSession, logout } from "@/lib/auth"

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

function AuthedLayout() {
  const { principal } = Route.useRouteContext()
  const navigate = useNavigate()
  const [pending, setPending] = useState(false)
  const [failed, setFailed] = useState(false)

  async function signOut() {
    setPending(true)
    setFailed(false)
    try {
      await logout()
      invalidateSession()
      await navigate({ to: "/login" })
    } catch {
      setFailed(true)
    } finally {
      setPending(false)
    }
  }

  return (
    <>
      <header className="flex items-center justify-between gap-4 border-b px-4 py-3 sm:px-6">
        <Link to="/" className="text-sm font-semibold">
          SlashObserve
        </Link>
        <div className="flex min-w-0 items-center gap-3 text-sm">
          <ThemeToggle />
          <span className="truncate max-sm:sr-only">
            {principal.email || principal.name}
          </span>
          {failed ? (
            <p role="alert" className="text-destructive">
              Sign out failed. Try again.
            </p>
          ) : null}
          <Button
            variant="outline"
            size="sm"
            disabled={pending}
            aria-busy={pending}
            onClick={signOut}
          >
            Sign out
          </Button>
        </div>
      </header>
      <Outlet />
    </>
  )
}
