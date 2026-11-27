import { useState, type RefObject } from "react"
import type { QueryKey } from "@tanstack/react-query"
import { LoaderCircleIcon } from "lucide-react"

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
import { sentence } from "@/observe/queries"

import { useDeleteMessage } from "./thread-queries"

function describeDeleteError(error: unknown): string {
  if (error instanceof ApiError && error.status < 500) {
    return sentence(error.message)
  }
  return "The comment could not be deleted. Try again."
}

export function DeleteMessageDialog({
  open,
  onOpenChange,
  messageId,
  wholeThread,
  queryKey,
  finalFocus,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  messageId: string
  wholeThread: boolean
  queryKey: QueryKey
  finalFocus: RefObject<HTMLElement | null>
}) {
  const [problem, setProblem] = useState<string | null>(null)
  const remove = useDeleteMessage(queryKey)

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
    remove.mutate(messageId, {
      onSuccess: () => onOpenChange(false),
      onError: (error) => setProblem(describeDeleteError(error)),
    })
  }

  return (
    <AlertDialog open={open} onOpenChange={changeOpen}>
      <AlertDialogContent finalFocus={finalFocus}>
        <AlertDialogHeader>
          <AlertDialogTitle>
            {wholeThread ? "Delete this thread?" : "Delete this comment?"}
          </AlertDialogTitle>
          <AlertDialogDescription>
            {wholeThread
              ? "The comment and the thread it started are deleted for everyone. This cannot be undone."
              : "The comment is deleted for everyone. This cannot be undone."}
          </AlertDialogDescription>
        </AlertDialogHeader>
        {problem === null ? null : (
          <Alert variant="destructive">
            <AlertDescription>{problem}</AlertDescription>
          </Alert>
        )}
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
            Delete
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
