import { useCallback, useId, useState } from "react"
import { Link, createFileRoute, useRouteContext } from "@tanstack/react-router"
import { FolderPlusIcon, PlusIcon } from "lucide-react"
import { toast } from "sonner"

import { PageHeader } from "@/components/page-header"
import { Button, buttonVariants } from "@/components/ui/button"
import { DashboardList } from "@/dashboards/dashboard-list"
import { validateListSearch, type ListSearch } from "@/dashboards/list-filters"
import { NewFolderDialog } from "@/folders/new-folder-dialog"
import { useDocumentTitle } from "@/lib/document-title"
import { roleAtLeast } from "@/lib/roles"
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
  const { principal } = useRouteContext({ from: "/_authed" })
  const [creatingFolder, setCreatingFolder] = useState(false)
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
          roleAtLeast(principal.role, "editor") ? (
            <>
              <Button variant="outline" onClick={() => setCreatingFolder(true)}>
                <FolderPlusIcon aria-hidden="true" />
                New folder
              </Button>
              <Link to="/" className={cn(buttonVariants())}>
                <PlusIcon aria-hidden="true" />
                Create dashboard
              </Link>
            </>
          ) : null
        }
      />
      <NewFolderDialog
        open={creatingFolder}
        onOpenChange={setCreatingFolder}
        onCreated={(folder) => toast.success(`Created folder ${folder.title}`)}
      />
      <DashboardList
        labelledBy={headingId}
        search={search}
        onSearchChange={onSearchChange}
      />
    </>
  )
}
