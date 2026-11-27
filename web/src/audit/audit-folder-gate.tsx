import type { ReactNode } from "react"
import { useQuery } from "@tanstack/react-query"

import { loadProblemText } from "@/api/errors"
import { EmptyState } from "@/components/empty-state"
import { LoadProblem } from "@/components/load-problem"
import { PageHeader } from "@/components/page-header"
import { Skeleton } from "@/components/ui/skeleton"
import { foldersQuery, type Folder } from "@/folders/queries"

import { auditFolderOf } from "./audit-folder"

export function AuditFolderGate({
  title,
  children,
}: {
  title: string
  children: (folder: Folder) => ReactNode
}) {
  const folders = useQuery(foldersQuery())
  const folder = auditFolderOf(folders.data)

  if (folder !== null) return children(folder)
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
      <PageHeader title={title} />
      <div className="rounded-card border bg-background">
        <EmptyState title="The Audit folder was deleted.">
          The server made it to hold the starter dashboards and does not create
          it again. The audit log still works.
        </EmptyState>
      </div>
    </>
  )
}
