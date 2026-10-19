import { useEffect, useRef, useState, type KeyboardEvent } from "react"

import { ApiError } from "@/api/errors"
import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"
import { useAddComment } from "@/comments/queries"
import { sentence } from "@/observe/queries"
import {
  describeRevisionError,
  maxPromptLength,
  useRequestRevision,
} from "@/revisions/queries"

import type { AgentGate } from "./agent-gate"
import { closeUnlessBusy, dismissLabel, runReply } from "./reply-flow"

type Props = {
  dashboardId: string
  rootId: string
  gate: AgentGate
  latestVersion: number
  onClose: () => void
}

function describeReplyError(error: unknown): string {
  if (error instanceof ApiError && error.status < 500) {
    return sentence(error.message)
  }
  return "The reply could not be sent. Try again."
}

export function ReplyForm({
  dashboardId,
  rootId,
  gate,
  latestVersion,
  onClose,
}: Props) {
  const [body, setBody] = useState("")
  const [error, setError] = useState<string | null>(null)
  const [replySaved, setReplySaved] = useState(false)
  const textarea = useRef<HTMLTextAreaElement>(null)
  const inFlight = useRef(false)
  const mounted = useRef(false)
  const addComment = useAddComment(dashboardId)
  const requestRevision = useRequestRevision(dashboardId)
  const text = body.trim()
  const busy = addComment.isPending || requestRevision.isPending
  const canSend = gate.kind === "allowed"

  useEffect(() => {
    mounted.current = true
    textarea.current?.focus()
    return () => {
      mounted.current = false
    }
  }, [])

  function dismiss() {
    closeUnlessBusy(busy || inFlight.current, onClose)
  }

  async function postReply(): Promise<boolean> {
    if (replySaved) return true
    try {
      await addComment.mutateAsync({ parentId: rootId, body: text })
      setReplySaved(true)
      return true
    } catch (failure) {
      setError(describeReplyError(failure))
      return false
    }
  }

  async function submit(sendToAgent: boolean) {
    if (inFlight.current || text === "" || (!sendToAgent && replySaved)) return
    inFlight.current = true
    setError(null)
    try {
      const outcome = await runReply({
        sendToAgent,
        postReply,
        sendRequest: async () => {
          await requestRevision.mutateAsync({
            prompt: text,
            baseVersion: latestVersion,
            commentId: rootId,
          })
        },
        isOpen: () => mounted.current,
        onClose,
        onRequestError: (failure) =>
          setError(`Your reply was saved. ${describeRevisionError(failure)}`),
      })
      if (outcome === "retry") textarea.current?.focus()
    } finally {
      inFlight.current = false
    }
  }

  function onKeyDown(event: KeyboardEvent<HTMLFormElement>) {
    if (event.key === "Escape") {
      event.preventDefault()
      event.stopPropagation()
      dismiss()
    } else if (
      event.key === "Enter" &&
      (event.metaKey || event.ctrlKey) &&
      !event.nativeEvent.isComposing
    ) {
      event.preventDefault()
      void submit(false)
    }
  }

  return (
    <form
      aria-label="Reply to this comment"
      className="mt-1 flex flex-col gap-2"
      onSubmit={(event) => {
        event.preventDefault()
        void submit(false)
      }}
      onKeyDown={onKeyDown}
    >
      <Textarea
        ref={textarea}
        aria-label="Reply"
        rows={2}
        maxLength={maxPromptLength}
        value={body}
        readOnly={busy || replySaved}
        onChange={(event) => setBody(event.target.value)}
      />
      {error === null ? null : (
        <p role="alert" className="text-xs text-destructive">
          {error}
        </p>
      )}
      {gate.kind === "blocked" ? (
        <p className="text-xs text-muted-foreground">{gate.reason}</p>
      ) : null}
      <div className="flex flex-wrap items-center justify-end gap-2">
        {replySaved && !canSend ? (
          <Button type="button" size="sm" onClick={dismiss}>
            {dismissLabel(replySaved)}
          </Button>
        ) : (
          <>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              aria-disabled={busy || undefined}
              onClick={dismiss}
            >
              {dismissLabel(replySaved)}
            </Button>
            <Button
              type="submit"
              variant="outline"
              size="sm"
              disabled={text === "" || replySaved}
              aria-disabled={busy || undefined}
            >
              Reply
            </Button>
            {canSend ? (
              <Button
                type="button"
                size="sm"
                disabled={text === ""}
                aria-disabled={busy || undefined}
                onClick={() => void submit(true)}
              >
                Send to agent
              </Button>
            ) : null}
          </>
        )}
      </div>
    </form>
  )
}
