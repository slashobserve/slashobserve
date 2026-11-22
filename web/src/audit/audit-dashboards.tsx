import { useQuery } from "@tanstack/react-query"

import { EmptyState } from "@/components/empty-state"
import { LoadProblem } from "@/components/load-problem"
import { PageHeader } from "@/components/page-header"
import { Skeleton } from "@/components/ui/skeleton"
import type { ListSearch } from "@/dashboards/list-filters"
import { FolderPage } from "@/folders/folder-page"
import { foldersQuery } from "@/folders/queries"
import { loadProblemText } from "@/service-accounts/queries"

import { auditFolderOf } from "./audit-folder"

export function AuditDashboards({
  search,
  onSearchChange,
  settingsOpen,
  onSettings,
}: {
  search: ListSearch
  onSearchChange: (next: ListSearch) => void
  settingsOpen: boolean
  onSettings: (open: boolean) => void
}) {
  const folders = useQuery(foldersQuery())
  const folder = auditFolderOf(folders.data)

  if (folder !== null) {
    return (
      <FolderPage
        key={folder.id}
        folderId={folder.id}
        search={search}
        onSearchChange={onSearchChange}
        settingsOpen={settingsOpen}
        onSettings={onSettings}
      />
    )
  }
  if (folders.isError) {
    return (
      <LoadProblem
        title="Could not load the Audit folder"
        detail={loadProblemText(folders.error)}
        retrying={folders.isFetching}
        onRetry={() => void folders.refetch()}
      />
    )
  }
  if (folders.isPending) {
    return <Skeleton className="h-12 w-64 max-w-full" />
  }
  return (
    <>
      <PageHeader title="Audit dashboards" />
      <div className="rounded-card border bg-background">
        <EmptyState title="The Audit folder was deleted.">
          The server made it to hold the starter dashboards and does not create
          it again. The audit log and Explore still work.
        </EmptyState>
      </div>
    </>
  )
}
