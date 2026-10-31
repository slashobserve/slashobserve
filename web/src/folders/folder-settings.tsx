import { useId, useState, type FormEvent } from "react"
import { Link } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { LoaderCircleIcon } from "lucide-react"
import { toast } from "sonner"

import { ApiError } from "@/api/errors"
import { LoadProblem } from "@/components/load-problem"
import { PageHeader } from "@/components/page-header"
import { RoleNote } from "@/components/role-note"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button, buttonVariants } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { useDocumentTitle } from "@/lib/document-title"
import { cn } from "@/lib/utils"
import { loadProblemText } from "@/service-accounts/queries"

import { DeleteFolderCard } from "./delete-folder-card"
import { FolderAccessCard } from "./folder-access-card"
import { levelLabels } from "./grants"
import { folderQuery, useRenameFolder, type Folder } from "./queries"

const maxTitleLength = 100

function description(folder: Folder): string {
  const count = `${folder.dashboard_count} ${folder.dashboard_count === 1 ? "dashboard" : "dashboards"}.`
  return folder.created_by === null
    ? count
    : `${count} Created by ${folder.created_by.name}.`
}

function renameProblemText(error: unknown): string {
  if (error instanceof ApiError && error.code === "folder_exists") {
    return "A folder with this name already exists."
  }
  return loadProblemText(error)
}

function isMissing(error: unknown): boolean {
  return (
    error instanceof ApiError && (error.status === 404 || error.status === 400)
  )
}

export function FolderSettings({ folderId }: { folderId: string }) {
  const query = useQuery(folderQuery(folderId))
  const folder = query.data
  useDocumentTitle(
    folder === undefined
      ? "Folder settings - SlashObserve"
      : `${folder.title} - Folder settings - SlashObserve`
  )

  if (folder === undefined) {
    if (query.isError && isMissing(query.error)) return <FolderNotFound />
    if (query.isError) {
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
        <PageHeader title="Folder settings" />
        <Skeleton className="h-24 max-w-[860px]" />
      </>
    )
  }

  const canManage = folder.permissions.manage
  return (
    <>
      <PageHeader title={folder.title} description={description(folder)} />
      {folder.general ? (
        <RoleNote className="py-2">
          General holds dashboards that are in no other folder. Everyone sees it
          by role.
        </RoleNote>
      ) : (
        <>
          {canManage ? null : (
            <RoleNote className="mb-4">
              {`Your access to this folder is ${levelLabels[folder.level]}. Only folder Admins can change it.`}
            </RoleNote>
          )}
          {canManage ? <NameCard folder={folder} /> : null}
          <FolderAccessCard folderId={folder.id} readOnly={!canManage} />
          {canManage ? <DeleteFolderCard folder={folder} /> : null}
        </>
      )}
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

function NameCard({ folder }: { folder: Folder }) {
  const [draft, setDraft] = useState(folder.title)
  const [problem, setProblem] = useState<string | null>(null)
  const headingId = useId()
  const inputId = useId()
  const rename = useRenameFolder(folder.id)
  const trimmed = draft.trim()
  const canRename =
    trimmed !== "" && trimmed !== folder.title && !rename.isPending

  function submit(event: FormEvent) {
    event.preventDefault()
    if (!canRename) return
    setProblem(null)
    rename.mutate(trimmed, {
      onSuccess: (renamed) => {
        setDraft(renamed.title)
        toast.success("Folder renamed")
      },
      onError: (error) => setProblem(renameProblemText(error)),
    })
  }

  return (
    <section
      aria-labelledby={headingId}
      className="mb-4 max-w-[860px] rounded-card border bg-background"
    >
      <div className="border-b px-4 py-3.5">
        <h2 id={headingId} className="text-sm font-semibold">
          Name
        </h2>
      </div>
      <form onSubmit={submit} className="flex flex-col gap-2 px-4 py-3">
        <div className="flex flex-wrap gap-2">
          <label htmlFor={inputId} className="sr-only">
            Folder name
          </label>
          <Input
            id={inputId}
            className="min-w-[240px] flex-1"
            maxLength={maxTitleLength}
            autoComplete="off"
            value={draft}
            onChange={(event) => {
              setDraft(event.target.value)
              setProblem(null)
            }}
          />
          <Button
            type="submit"
            disabled={!canRename}
            aria-busy={rename.isPending}
          >
            {rename.isPending ? (
              <LoaderCircleIcon
                aria-hidden="true"
                className="motion-safe:animate-spin"
              />
            ) : null}
            Rename
          </Button>
        </div>
        {problem === null ? null : (
          <Alert variant="destructive">
            <AlertDescription>{problem}</AlertDescription>
          </Alert>
        )}
      </form>
    </section>
  )
}
