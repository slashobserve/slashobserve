import { createFileRoute } from "@tanstack/react-router"
import { useId } from "react"

import { DashboardList } from "@/dashboards/dashboard-list"

export const Route = createFileRoute("/_authed/")({ component: Home })

function Home() {
  const headingId = useId()
  return (
    <main className="mx-auto w-full max-w-5xl p-6">
      <header className="mb-6 flex flex-col gap-1">
        <h1 id={headingId} className="text-2xl font-semibold">
          Dashboards
        </h1>
        <p className="text-sm text-muted-foreground">
          Every dashboard, most recently updated first.
        </p>
      </header>
      <DashboardList labelledBy={headingId} />
    </main>
  )
}
