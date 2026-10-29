import { useRef, useState } from "react"
import { useNavigate } from "@tanstack/react-router"
import { LoaderCircleIcon } from "lucide-react"
import { toast } from "sonner"

import { ApiError } from "@/api/errors"
import { Alert, AlertDescription } from "@/components/ui/alert"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Button } from "@/components/ui/button"
import { loadProblemText } from "@/service-accounts/queries"

import { useDeleteFolder, type Folder } from "./queries"

function problemText(error: unknown): string {
  if (error instanceof ApiError && error.code === "folder_not_empty") {
    return "This folder still holds dashboards. Move them out first."
  }
  if (error instanceof ApiError && error.status === 404) {
    return "This folder no longer exists."
  }
  return loadProblemText(error)
}

export function DeleteFolderCard({ folder }: { folder: Folder }) {
  const navigate = useNavigate()
  const trigger = useRef<HTMLButtonElement>(null)
  const remove = useDeleteFolder(folder.id)
  const [open, setOpen] = useState(false)
  const [problem, setProblem] = useState<string | null>(null)
  const empty = folder.dashboard_count === 0

  function changeOpen(next: boolean) {
    if (!next && remove.isPending) return
    setOpen(next)
    if (!next) {
      setProblem(null)
      remove.reset()
    }
  }

  function confirm() {
    setProblem(null)
    remove.mutate(undefined, {
      onSuccess: () => {
        toast.success(`Deleted ${folder.title}`)
        void navigate({ to: "/dashboards" })
      },
      onError: (error) => setProblem(problemText(error)),
    })
  }

  return (
    <section
      aria-labelledby="delete-folder-heading"
      className="mb-4 flex max-w-[860px] flex-wrap items-center justify-between gap-x-4 gap-y-2.5 rounded-card border bg-background px-4 py-3.5"
    >
      <div className="flex min-w-0 flex-col">
        <h2 id="delete-folder-heading" className="text-sm font-semibold">
          Delete folder
        </h2>
        <p className="text-xs text-muted-foreground">
          {empty
            ? "Deleting never deletes dashboards."
            : `Move its ${folder.dashboard_count} ${folder.dashboard_count === 1 ? "dashboard" : "dashboards"} out first. Deleting never deletes dashboards.`}
        </p>
      </div>
      <Button
        ref={trigger}
        variant="outline"
        disabled={!empty}
        onClick={() => setOpen(true)}
      >
        Delete folder
      </Button>
      <AlertDialog open={open} onOpenChange={changeOpen}>
        <AlertDialogContent finalFocus={trigger}>
          <AlertDialogHeader>
            <AlertDialogTitle className="wrap-anywhere">
              Delete {folder.title}?
            </AlertDialogTitle>
            <AlertDialogDescription>
              The folder and who can see it are removed. This cannot be undone.
            </AlertDialogDescription>
          </AlertDialogHeader>
          {problem !== null ? (
            <Alert variant="destructive">
              <AlertDescription>{problem}</AlertDescription>
            </Alert>
          ) : null}
          <AlertDialogFooter>
            <AlertDialogCancel aria-disabled={remove.isPending}>
              Cancel
            </AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              disabled={remove.isPending}
              focusableWhenDisabled
              aria-busy={remove.isPending}
              onClick={confirm}
            >
              {remove.isPending ? (
                <LoaderCircleIcon
                  aria-hidden="true"
                  className="motion-safe:animate-spin"
                />
              ) : null}
              Delete folder
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </section>
  )
}
