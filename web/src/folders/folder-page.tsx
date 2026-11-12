import { useId, useRef } from "react"
import { Link } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { SettingsIcon } from "lucide-react"

import { ApiError } from "@/api/errors"
import { LoadProblem } from "@/components/load-problem"
import { PageHeader } from "@/components/page-header"
import { Button, buttonVariants } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { DashboardList } from "@/dashboards/dashboard-list"
import type { ListSearch } from "@/dashboards/list-filters"
import { useDocumentTitle } from "@/lib/document-title"
import { cn } from "@/lib/utils"
import { loadProblemText } from "@/service-accounts/queries"
import { SidePanel } from "@/shell/side-panel"

import { FolderObservations } from "./folder-observations"
import { FolderSavedSearches } from "./folder-saved-searches"
import { FolderSettingsBody } from "./folder-settings"
import { folderQuery, type Folder } from "./queries"

type Props = {
  folderId: string
  search: ListSearch
  onSearchChange: (next: ListSearch) => void
  settingsOpen: boolean
  onSettings: (open: boolean) => void
}

function description(folder: Folder): string {
  const count = `${folder.dashboard_count} ${folder.dashboard_count === 1 ? "dashboard" : "dashboards"}.`
  return folder.created_by === null
    ? count
    : `${count} Created by ${folder.created_by.name}.`
}

function isMissing(error: unknown): boolean {
  return (
    error instanceof ApiError && (error.status === 404 || error.status === 400)
  )
}

export function FolderPage({
  folderId,
  search,
  onSearchChange,
  settingsOpen,
  onSettings,
}: Props) {
  const headingId = useId()
  const settingsButton = useRef<HTMLButtonElement>(null)
  const query = useQuery(folderQuery(folderId))
  const folder = query.data
  useDocumentTitle(
    folder === undefined
      ? "Folder - SlashObserve"
      : `${folder.title} - SlashObserve`
  )

  if (folder === undefined && query.isError) {
    if (isMissing(query.error)) return <FolderNotFound />
    return (
      <LoadProblem
        title="Could not load this folder"
        detail={loadProblemText(query.error)}
        retrying={query.isFetching}
        onRetry={() => void query.refetch()}
      />
    )
  }

  return (
    <>
      {folder === undefined ? (
        <Skeleton className="mb-4 h-12 w-64 max-w-full" />
      ) : (
        <PageHeader
          headingId={headingId}
          title={folder.title}
          description={description(folder)}
          actions={
            folder.permissions.manage ? (
              <Button
                ref={settingsButton}
                variant="outline"
                aria-expanded={settingsOpen}
                onClick={() => onSettings(true)}
              >
                <SettingsIcon aria-hidden="true" />
                Folder settings
              </Button>
            ) : null
          }
        />
      )}
      <DashboardList
        labelledBy={headingId}
        search={search}
        onSearchChange={onSearchChange}
        folderId={folderId}
      />
      {folder === undefined ? null : (
        <>
          <FolderObservations folderId={folderId} />
          <FolderSavedSearches folderId={folderId} />
        </>
      )}
      {folder !== undefined && settingsOpen ? (
        <SidePanel
          title="Folder settings"
          description={`Name, access and deletion for ${folder.title}.`}
          onClose={() => onSettings(false)}
          trigger={settingsButton}
        >
          <FolderSettingsBody folder={folder} />
        </SidePanel>
      ) : null}
    </>
  )
}

function FolderNotFound() {
  return (
    <div className="flex flex-col items-center justify-center gap-4 p-6 text-center">
      <h1 className="text-2xl font-semibold">Folder not found</h1>
      <p className="text-sm text-muted-foreground">
        Folder not found. It may have been deleted, or you do not have access.
      </p>
      <Link
        to="/dashboards"
        className={cn(buttonVariants({ variant: "outline" }))}
      >
        Back to dashboards
      </Link>
    </div>
  )
}
