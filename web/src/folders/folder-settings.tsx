import { useId, useState, type FormEvent } from "react"
import { LoaderCircleIcon } from "lucide-react"
import { toast } from "sonner"

import { ApiError } from "@/api/errors"
import { RoleNote } from "@/components/role-note"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { loadProblemText } from "@/service-accounts/queries"

import { DeleteFolderCard } from "./delete-folder-card"
import { FolderAccessCard } from "./folder-access-card"
import { levelLabels } from "./grants"
import { useRenameFolder, type Folder } from "./queries"

const maxTitleLength = 100

function renameProblemText(error: unknown): string {
  if (error instanceof ApiError && error.code === "folder_exists") {
    return "A folder with this name already exists."
  }
  return loadProblemText(error)
}

export function FolderSettingsBody({ folder }: { folder: Folder }) {
  if (folder.general) {
    return (
      <RoleNote className="py-2">
        General holds dashboards that are in no other folder. Everyone sees it
        by role.
      </RoleNote>
    )
  }
  if (folder.personal) {
    return (
      <RoleNote className="py-2">
        Only you can see My Dashboards, and Admins cannot. Every new dashboard
        you make starts here. To share one, publish it to a folder where you
        have Edit from its Share pane.
      </RoleNote>
    )
  }
  const canManage = folder.permissions.manage
  return (
    <>
      {canManage ? null : (
        <RoleNote>
          {`Your access to this folder is ${levelLabels[folder.level]}. Only folder Admins can change it.`}
        </RoleNote>
      )}
      {canManage ? <NameCard folder={folder} /> : null}
      <FolderAccessCard folderId={folder.id} readOnly={!canManage} />
      {canManage ? <DeleteFolderCard folder={folder} /> : null}
    </>
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
      className="rounded-card border bg-background"
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
