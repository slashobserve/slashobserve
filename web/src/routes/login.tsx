import { useEffect } from "react"
import { createFileRoute, isRedirect, redirect } from "@tanstack/react-router"

import { buttonVariants } from "@/components/ui/button"
import { forgetHomeChecked } from "@/home/home-dashboard"
import { getSession, loginHref, sessionEnded } from "@/lib/auth"
import { cn } from "@/lib/utils"

type LoginSearch = {
  return_to?: string | undefined
  error?: string | undefined
}

const errorMessages: Record<string, string> = {
  access_denied: "Sign-in was cancelled.",
  email_not_allowed: "This account is not allowed to sign in.",
  account_deactivated:
    "This account was deactivated by an Admin. Ask an Admin to reactivate it.",
  unavailable: "The identity provider is unavailable. Try again shortly.",
  session_unavailable: "SlashObserve is unavailable. Try again shortly.",
  sign_in_off:
    "Browser sign-in is not set up on this server. Use a service-account key, or ask the operator to configure an identity provider.",
}

const maxReturnToLength = 2048

function localPath(path: string | undefined): string {
  if (path === undefined || path.length > maxReturnToLength) return "/"
  if (!path.startsWith("/") || path[1] === "/") return "/"
  for (let i = 0; i < path.length; i++) {
    const code = path.charCodeAt(i)
    if (code <= 0x1f || code === 0x7f || code === 0x5c) return "/"
  }
  return path
}

export const Route = createFileRoute("/login")({
  validateSearch: (search: Record<string, unknown>): LoginSearch => ({
    return_to:
      typeof search.return_to === "string" ? search.return_to : undefined,
    error: typeof search.error === "string" ? search.error : undefined,
  }),
  beforeLoad: async ({ search }) => {
    try {
      const session = await getSession()
      if (session.authenticated) {
        throw redirect({ href: localPath(search.return_to) })
      }
      forgetHomeChecked()
    } catch (error) {
      if (isRedirect(error)) throw error
      return { sessionUnavailable: true, sessionEnded: false }
    }
    return { sessionUnavailable: false, sessionEnded: sessionEnded() }
  },
  component: Login,
})

function Login() {
  const { return_to, error } = Route.useSearch()
  const { sessionUnavailable, sessionEnded } = Route.useRouteContext()
  const code = error ?? (sessionUnavailable ? "session_unavailable" : undefined)

  useEffect(() => {
    const previous = document.title
    document.title = "Sign in - SlashObserve"
    return () => {
      document.title = previous
    }
  }, [])

  return (
    <main className="flex flex-1 flex-col items-center justify-center gap-3 p-6 text-center">
      <h1 className="text-3xl font-semibold tracking-tight">SlashObserve</h1>
      <p className="max-w-md text-sm text-muted-foreground">
        Live dashboards from a single prompt.
      </p>
      {sessionEnded && !code ? (
        <p role="status" className="max-w-md text-sm">
          Your session was ended, so you were signed out. Sign in again to carry
          on.
        </p>
      ) : null}
      {code ? (
        <p role="alert" className="text-sm text-destructive">
          {Object.hasOwn(errorMessages, code)
            ? errorMessages[code]
            : "Sign-in failed. Try again."}
        </p>
      ) : null}
      <a
        href={loginHref(localPath(return_to))}
        className={cn(buttonVariants())}
      >
        Sign in
      </a>
    </main>
  )
}
