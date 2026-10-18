import { useId } from "react"
import { Link, createFileRoute } from "@tanstack/react-router"
import { PlusIcon } from "lucide-react"

import { PageHeader } from "@/components/page-header"
import { buttonVariants } from "@/components/ui/button"
import { DashboardList } from "@/dashboards/dashboard-list"
import { useDocumentTitle } from "@/lib/document-title"
import { cn } from "@/lib/utils"

export const Route = createFileRoute("/_authed/_shell/dashboards/")({
  staticData: { crumbs: [{ label: "Dashboards" }] },
  component: DashboardsPage,
})

function DashboardsPage() {
  const headingId = useId()
  useDocumentTitle("Dashboards - SlashObserve")
  return (
    <div className="mx-auto w-full max-w-5xl">
      <PageHeader
        headingId={headingId}
        title="Dashboards"
        actions={
          <Link to="/" className={cn(buttonVariants())}>
            <PlusIcon aria-hidden="true" />
            Create dashboard
          </Link>
        }
      />
      <DashboardList labelledBy={headingId} />
    </div>
  )
}
