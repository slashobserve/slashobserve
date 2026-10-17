import { createFileRoute, redirect } from "@tanstack/react-router"

import { buttonVariants } from "@/components/ui/button"
import { getSession, loginHref } from "@/lib/auth"

type LoginSearch = { return_to?: string; error?: string }

const errorMessages: Record<string, string> = {
  access_denied: "Sign-in was cancelled.",
  email_not_allowed: "This account is not allowed to sign in.",
  unavailable: "The identity provider is unavailable. Try again shortly.",
}

function localPath(path: string | undefined): string {
  return path !== undefined && path.startsWith("/") && !path.startsWith("//")
    ? path
    : "/"
}

export const Route = createFileRoute("/login")({
  validateSearch: (search: Record<string, unknown>): LoginSearch => ({
    return_to:
      typeof search.return_to === "string" ? search.return_to : undefined,
    error: typeof search.error === "string" ? search.error : undefined,
  }),
  beforeLoad: async ({ search }) => {
    const session = await getSession()
    if (session.authenticated) {
      throw redirect({ href: localPath(search.return_to) })
    }
  },
  component: Login,
})

function Login() {
  const { return_to, error } = Route.useSearch()
  return (
    <main className="flex flex-1 flex-col items-center justify-center gap-3 p-6 text-center">
      <h1 className="text-3xl font-semibold tracking-tight">SlashObserve</h1>
      <p className="max-w-md text-sm text-muted-foreground">
        Live dashboards from a single prompt.
      </p>
      {error ? (
        <p role="alert" className="text-sm text-destructive">
          {errorMessages[error] ?? "Sign-in failed. Try again."}
        </p>
      ) : null}
      <a href={loginHref(localPath(return_to))} className={buttonVariants()}>
        Sign in
      </a>
    </main>
  )
}
