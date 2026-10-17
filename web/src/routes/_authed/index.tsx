import { createFileRoute } from "@tanstack/react-router"

export const Route = createFileRoute("/_authed/")({ component: Home })

function Home() {
  return (
    <main className="flex flex-1 flex-col items-center justify-center gap-3 p-6 text-center">
      <h1 className="text-3xl font-semibold tracking-tight">SlashObserve</h1>
      <p className="max-w-md text-sm text-muted-foreground">
        Live dashboards from a single prompt.
      </p>
    </main>
  )
}
