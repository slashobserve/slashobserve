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
  const addComment = useAddComment(dashboardId)
  const requestRevision = useRequestRevision(dashboardId)
  const text = body.trim()
  const busy = addComment.isPending || requestRevision.isPending
  const blocked = text === "" || busy

  useEffect(() => {
    textarea.current?.focus()
  }, [])

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

  async function reply() {
    if (blocked || replySaved) return
    setError(null)
    if (await postReply()) onClose()
  }

  async function sendToAgent() {
    if (blocked) return
    setError(null)
    if (!(await postReply())) return
    try {
      await requestRevision.mutateAsync({
        prompt: text,
        baseVersion: latestVersion,
        commentId: rootId,
      })
    } catch (failure) {
      setError(`Your reply was saved. ${describeRevisionError(failure)}`)
      return
    }
    onClose()
  }

  function onKeyDown(event: KeyboardEvent<HTMLFormElement>) {
    if (event.key === "Escape") {
      event.preventDefault()
      event.stopPropagation()
      onClose()
    } else if (
      event.key === "Enter" &&
      (event.metaKey || event.ctrlKey) &&
      !event.nativeEvent.isComposing
    ) {
      event.preventDefault()
      reply()
    }
  }

  return (
    <form
      aria-label="Reply to this comment"
      className="mt-2.5 flex flex-col gap-2"
      onSubmit={(event) => {
        event.preventDefault()
        reply()
      }}
      onKeyDown={onKeyDown}
    >
      <Textarea
        ref={textarea}
        aria-label="Reply"
        rows={2}
        maxLength={maxPromptLength}
        value={body}
        readOnly={replySaved}
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
        <Button type="button" variant="ghost" size="sm" onClick={onClose}>
          Cancel
        </Button>
        <Button
          type="submit"
          variant="outline"
          size="sm"
          disabled={blocked || replySaved}
        >
          Reply
        </Button>
        {gate.kind === "allowed" ? (
          <Button
            type="button"
            size="sm"
            disabled={blocked}
            onClick={sendToAgent}
          >
            Send to agent
          </Button>
        ) : null}
      </div>
    </form>
  )
}
