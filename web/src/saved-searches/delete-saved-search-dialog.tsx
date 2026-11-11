import { useState } from "react"
import { LoaderCircleIcon } from "lucide-react"
import { toast } from "sonner"

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
import { loadProblemText } from "@/service-accounts/queries"

import { useDeleteSavedSearch, type SavedSearch } from "./queries"

type Props = {
  saved: SavedSearch
  open: boolean
  onOpenChange: (open: boolean) => void
  onDeleted: () => void
  finalFocus: () => HTMLElement | boolean
}

export function DeleteSavedSearchDialog({
  saved,
  open,
  onOpenChange,
  onDeleted,
  finalFocus,
}: Props) {
  const remove = useDeleteSavedSearch(saved.id)
  const [problem, setProblem] = useState<string | null>(null)

  function changeOpen(next: boolean) {
    if (!next && remove.isPending) return
    onOpenChange(next)
    if (!next) {
      setProblem(null)
      remove.reset()
    }
  }

  function confirm() {
    setProblem(null)
    remove.mutate(undefined, {
      onSuccess: () => {
        toast.success(`Deleted ${saved.title}`)
        onDeleted()
        onOpenChange(false)
      },
      onError: (error) => setProblem(loadProblemText(error)),
    })
  }

  return (
    <AlertDialog open={open} onOpenChange={changeOpen}>
      <AlertDialogContent finalFocus={finalFocus}>
        <AlertDialogHeader>
          <AlertDialogTitle className="wrap-anywhere">
            Delete {saved.title}?
          </AlertDialogTitle>
          <AlertDialogDescription>
            Everyone who can view {saved.folder.title} loses this saved search.
            The deletion stays in the activity log.
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
            Delete saved search
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
