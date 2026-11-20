import { useId, useState, type FormEvent } from "react"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { cn } from "@/lib/utils"

import {
  maxNoteLength,
  noteProblem,
  outcomeOf,
  outcomeText,
  useRequestCommentAccess,
  type AccessObject,
  type RequestOutcome,
} from "./request-access"

export function RequestCommentAccess({
  object,
  className,
}: {
  object: AccessObject
  className?: string
}) {
  const noteId = useId()
  const request = useRequestCommentAccess(object)
  const [composing, setComposing] = useState(false)
  const [note, setNote] = useState("")
  const [outcome, setOutcome] = useState<RequestOutcome | null>(null)
  const problem = noteProblem(note)

  if (outcome !== null && outcome !== "failed") {
    return (
      <p role="status" className={cn("text-[13px]", className)}>
        {outcomeText(outcome)}
      </p>
    )
  }

  if (!composing) {
    return (
      <Button
        variant="outline"
        size="sm"
        className={cn("self-start", className)}
        onClick={() => setComposing(true)}
      >
        Request comment access
      </Button>
    )
  }

  function send(event: FormEvent) {
    event.preventDefault()
    if (problem !== null || request.isPending) return
    request.mutate(note, {
      onSuccess: () => setOutcome("sent"),
      onError: (error) => setOutcome(outcomeOf(error)),
    })
  }

  return (
    <form onSubmit={send} className={cn("flex flex-col gap-2", className)}>
      <label htmlFor={noteId} className="text-xs text-muted-foreground">
        Note for the people who can grant access (optional)
      </label>
      <Input
        id={noteId}
        value={note}
        maxLength={maxNoteLength}
        onChange={(event) => setNote(event.target.value)}
        aria-invalid={problem !== null || undefined}
        autoFocus
      />
      {problem === null ? null : (
        <p className="text-xs text-destructive">{problem}</p>
      )}
      {outcome === "failed" ? (
        <p role="alert" className="text-xs text-destructive">
          {outcomeText("failed")} Try again.
        </p>
      ) : null}
      <div className="flex gap-1.5">
        <Button
          type="submit"
          size="sm"
          disabled={problem !== null || request.isPending}
          aria-busy={request.isPending || undefined}
        >
          {request.isPending ? "Sending" : "Send request"}
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={() => {
            setComposing(false)
            setOutcome(null)
          }}
        >
          Cancel
        </Button>
      </div>
    </form>
  )
}
