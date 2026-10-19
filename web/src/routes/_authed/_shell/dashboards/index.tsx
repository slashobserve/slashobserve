import { useCallback, useId } from "react"
import { Link, createFileRoute } from "@tanstack/react-router"
import { PlusIcon } from "lucide-react"

import { PageHeader } from "@/components/page-header"
import { buttonVariants } from "@/components/ui/button"
import { DashboardList } from "@/dashboards/dashboard-list"
import { validateListSearch, type ListSearch } from "@/dashboards/list-filters"
import { useDocumentTitle } from "@/lib/document-title"
import { cn } from "@/lib/utils"

export const Route = createFileRoute("/_authed/_shell/dashboards/")({
  staticData: { crumbs: [{ label: "Dashboards" }] },
  validateSearch: validateListSearch,
  component: DashboardsPage,
})

function DashboardsPage() {
  const headingId = useId()
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  const onSearchChange = useCallback(
    (next: ListSearch) => void navigate({ search: next, replace: true }),
    [navigate]
  )
  useDocumentTitle("Dashboards - SlashObserve")
  return (
    <>
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
      <DashboardList
        labelledBy={headingId}
        search={search}
        onSearchChange={onSearchChange}
      />
    </>
  )
}
