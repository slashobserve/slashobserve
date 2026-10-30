import { useState } from "react"
import { LoaderCircleIcon } from "lucide-react"

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

import { deleteDescription, deleteOutcome } from "./delete-dashboard"
import { useDeleteDashboard } from "./queries"

type Props = {
  dashboard: { id: string; title: string; versions: number }
  open: boolean
  onOpenChange: (open: boolean) => void
  onDeleted: (alreadyGone: boolean) => void | Promise<void>
  finalFocus: () => HTMLElement | boolean
}

export function DeleteDashboardDialog({
  dashboard,
  open,
  onOpenChange,
  onDeleted,
  finalFocus,
}: Props) {
  const remove = useDeleteDashboard(dashboard.id)
  const [problem, setProblem] = useState<string | null>(null)

  function changeOpen(next: boolean) {
    if (!next && remove.isPending) return
    onOpenChange(next)
    if (!next) {
      setProblem(null)
      remove.reset()
    }
  }

  async function confirm() {
    setProblem(null)
    let alreadyGone = false
    try {
      await remove.mutateAsync()
    } catch (error) {
      const outcome = deleteOutcome(error)
      if (outcome.kind === "problem") {
        setProblem(outcome.text)
        return
      }
      alreadyGone = true
    }
    await onDeleted(alreadyGone)
  }

  return (
    <AlertDialog open={open} onOpenChange={changeOpen}>
      <AlertDialogContent finalFocus={finalFocus}>
        <AlertDialogHeader>
          <AlertDialogTitle className="wrap-anywhere">
            Delete {dashboard.title}?
          </AlertDialogTitle>
          <AlertDialogDescription>
            {deleteDescription(dashboard.versions)} To keep a copy, export the
            spec JSON from the dashboard&apos;s More menu first.
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
            disabled={remove.isPending || remove.isSuccess}
            focusableWhenDisabled
            aria-busy={remove.isPending}
            onClick={() => void confirm()}
          >
            {remove.isPending ? (
              <LoaderCircleIcon
                aria-hidden="true"
                className="motion-safe:animate-spin"
              />
            ) : null}
            Delete dashboard
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
