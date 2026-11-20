import { useRef, useState } from "react"
import type { QueryKey } from "@tanstack/react-query"
import { LoaderCircleIcon, Trash2Icon } from "lucide-react"

import { EntryAction } from "@/activity/entry-frame"
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

import { useDeleteMessage } from "./queries"

function describeDeleteError(error: unknown): string {
  if (error instanceof ApiError && error.status < 500) {
    return sentence(error.message)
  }
  return "The comment could not be deleted. Try again."
}

export function DeleteMessageButton({
  messageId,
  wholeThread,
  queryKey,
}: {
  messageId: string
  wholeThread: boolean
  queryKey: QueryKey
}) {
  const [open, setOpen] = useState(false)
  const [problem, setProblem] = useState<string | null>(null)
  const trigger = useRef<HTMLButtonElement>(null)
  const remove = useDeleteMessage(queryKey)

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
    remove.mutate(messageId, {
      onSuccess: () => setOpen(false),
      onError: (error) => setProblem(describeDeleteError(error)),
    })
  }

  return (
    <>
      <EntryAction
        ref={trigger}
        icon={<Trash2Icon aria-hidden="true" />}
        onClick={() => setOpen(true)}
      >
        Delete
      </EntryAction>
      <AlertDialog open={open} onOpenChange={changeOpen}>
        <AlertDialogContent finalFocus={trigger}>
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
    </>
  )
}
