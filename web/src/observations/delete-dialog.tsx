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

import { useDeleteObservation } from "./queries"

type DeleteTarget = { id: string; title: string }

type Props = {
  targets: readonly DeleteTarget[]
  open: boolean
  onOpenChange: (open: boolean) => void
  onDeleted: (ids: readonly string[]) => void
}

export function deleteFailureText(error: unknown): string {
  return error instanceof Error && error.message !== ""
    ? error.message
    : "It could not be deleted. Try again."
}

export function DeleteObservationsDialog({
  targets,
  open,
  onOpenChange,
  onDeleted,
}: Props) {
  const remove = useDeleteObservation()
  const [problem, setProblem] = useState<string | null>(null)
  const [pending, setPending] = useState(false)
  const single = targets.length === 1 ? targets[0] : undefined

  function change(next: boolean) {
    if (!next && pending) return
    if (!next) setProblem(null)
    onOpenChange(next)
  }

  async function confirm() {
    setPending(true)
    setProblem(null)
    const done: string[] = []
    let failure: unknown = null
    for (const target of targets) {
      try {
        await remove.mutateAsync(target.id)
        done.push(target.id)
      } catch (error) {
        failure = error
      }
    }
    setPending(false)
    if (done.length > 0) {
      onDeleted(done)
      toast.success(
        done.length === 1
          ? "Observation deleted"
          : `${done.length} observations deleted`
      )
    }
    if (failure === null) {
      onOpenChange(false)
      return
    }
    const left = targets.length - done.length
    setProblem(
      `${left === 1 ? "One observation" : `${left} observations`} could not be deleted: ${deleteFailureText(failure)}`
    )
  }

  return (
    <AlertDialog open={open} onOpenChange={change}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>
            {single === undefined
              ? `Delete ${targets.length} observations?`
              : "Delete this observation?"}
          </AlertDialogTitle>
          <AlertDialogDescription>
            {single === undefined ? "They are" : `"${single.title}" is`} deleted
            for everyone, with the blocks, the discussion and the published RCA
            revisions. A public link to a revision stops working. This cannot be
            undone.
          </AlertDialogDescription>
        </AlertDialogHeader>
        {problem === null ? null : (
          <Alert variant="destructive">
            <AlertDescription>{problem}</AlertDescription>
          </Alert>
        )}
        <AlertDialogFooter>
          <AlertDialogCancel aria-disabled={pending}>Cancel</AlertDialogCancel>
          <AlertDialogAction
            variant="destructive"
            disabled={pending}
            focusableWhenDisabled
            aria-busy={pending}
            onClick={() => void confirm()}
          >
            {pending ? (
              <LoaderCircleIcon
                aria-hidden="true"
                className="motion-safe:animate-spin"
              />
            ) : null}
            {single === undefined
              ? "Delete observations"
              : "Delete observation"}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
