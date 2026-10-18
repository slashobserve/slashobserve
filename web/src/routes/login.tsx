import { useEffect } from "react"
import { createFileRoute, isRedirect, redirect } from "@tanstack/react-router"

import { buttonVariants } from "@/components/ui/button"
import { getSession, loginHref } from "@/lib/auth"
import { cn } from "cn"

type LoginSearch = { return_to?: string; error?: string }

const errorMessages: Record<string, string> = {
  access_denied: "Sign-in was cancelled.",
  email_not_allowed: "This account is not allowed to sign in.",
  unavailable: "The identity provider is unavailable. Try again shortly.",
  session_unavailable: "SlashObserve is unavailable. Try again shortly.",
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
    } catch (error) {
      if (isRedirect(error)) throw error
      return { sessionUnavailable: true }
    }
    return { sessionUnavailable: false }
  },
  component: Login,
})

function Login() {
  const { return_to, error } = Route.useSearch()
  const { sessionUnavailable } = Route.useRouteContext()
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
