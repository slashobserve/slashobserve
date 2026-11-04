import { useId, useState } from "react"
import { toast } from "sonner"

import { ApiError } from "@/api/errors"
import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"

import {
  canContinueBuild,
  cancelErrorText,
  continueErrorText,
  continueNoteError,
} from "./build"
import {
  useCancelRequest,
  useContinueRequest,
  type ObserveRequest,
} from "./queries"

function errorParts(error: unknown): { code: string; message: string } {
  return error instanceof ApiError
    ? { code: error.code, message: error.message }
    : { code: "", message: "" }
}

export function CancelRequestButton({
  request,
}: {
  request: Pick<ObserveRequest, "id" | "can_cancel" | "cancel_requested_at">
}) {
  const cancel = useCancelRequest(request.id)
  if (!request.can_cancel) return null
  const canceling = request.cancel_requested_at !== null || cancel.isPending
  return (
    <Button
      variant="outline"
      size="sm"
      disabled={canceling}
      aria-busy={cancel.isPending}
      onClick={() =>
        cancel.mutate(undefined, {
          onError: (error) => {
            const { code, message } = errorParts(error)
            toast.error(cancelErrorText(code, message))
          },
        })
      }
    >
      {canceling ? "Canceling" : "Cancel run"}
    </Button>
  )
}

export function ContinueBuildButton({
  request,
  isRequester,
  onContinued,
}: {
  request: Pick<
    ObserveRequest,
    "id" | "status" | "build" | "build_saved_at" | "continued_by"
  >
  isRequester: boolean
  onContinued: (next: ObserveRequest) => void
}) {
  const [open, setOpen] = useState(false)
  const [note, setNote] = useState("")
  const noteId = useId()
  const errorId = useId()
  const proceed = useContinueRequest(request.id)
  const allowed = canContinueBuild({
    status: request.status,
    build: request.build,
    buildSavedAt: request.build_saved_at,
    continuedBy: request.continued_by,
    isRequester,
  })
  if (!allowed) return null
  const noteError = continueNoteError(note)
  const busy = proceed.isPending

  function close() {
    setOpen(false)
    setNote("")
  }

  return (
    <div className="flex w-full max-w-xl flex-col items-start gap-2 sm:w-auto">
      <Button
        variant="outline"
        size="sm"
        aria-expanded={open}
        disabled={busy}
        onClick={() => (open ? close() : setOpen(true))}
      >
        Continue
      </Button>
      {open ? (
        <form
          className="flex w-full flex-col gap-2"
          onSubmit={(event) => {
            event.preventDefault()
            if (noteError !== null || busy) return
            proceed.mutate(
              { note },
              {
                onSuccess: (next) => {
                  close()
                  onContinued(next)
                },
                onError: (error) => {
                  const { code, message } = errorParts(error)
                  toast.error(continueErrorText(code, message))
                },
              }
            )
          }}
        >
          <Label htmlFor={noteId}>Note for the agent (optional)</Label>
          <Textarea
            id={noteId}
            value={note}
            onChange={(event) => setNote(event.target.value)}
            aria-invalid={noteError !== null}
            aria-describedby={noteError === null ? undefined : errorId}
            disabled={busy}
          />
          {noteError !== null ? (
            <p id={errorId} role="alert" className="text-destructive">
              {noteError}
            </p>
          ) : null}
          <div className="flex flex-wrap items-center gap-2">
            <Button
              type="submit"
              size="sm"
              disabled={noteError !== null || busy}
              aria-busy={busy}
            >
              {busy ? "Continuing" : "Continue build"}
            </Button>
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={busy}
              onClick={close}
            >
              Cancel
            </Button>
          </div>
        </form>
      ) : null}
    </div>
  )
}
