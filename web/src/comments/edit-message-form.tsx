import { useEffect, useRef, useState, type KeyboardEvent } from "react"
import type { QueryKey } from "@tanstack/react-query"

import { ApiError } from "@/api/errors"
import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"
import { useCoarsePointer } from "@/lib/coarse-pointer"
import { isModEnter, sendsOnEnter } from "@/lib/shortcuts"
import { cleanText } from "@/lib/user-text"
import { sentence } from "@/observe/queries"
import { maxPromptLength } from "@/revisions/queries"

import { MentionButton } from "./mention-button"
import { toDisplay, type MentionAgent, type MentionPerson } from "./mentions"
import { useEditMessage } from "./thread-queries"
import { SendHint } from "./send-hint"
import { useInlineMention } from "./use-inline-mention"
import { useMentionPicks } from "./use-mention-picks"

const noAgents: readonly MentionAgent[] = []

function describeEditError(error: unknown): string {
  if (error instanceof ApiError && error.status < 500) {
    return sentence(error.message)
  }
  return "The comment could not be saved. Try again."
}

export function EditMessageForm({
  message,
  people,
  target,
  queryKey,
  onClose,
}: {
  message: { id: string; body: string }
  people: readonly MentionPerson[]
  target: string | null
  queryKey: QueryKey
  onClose: () => void
}) {
  const [shown] = useState(() => toDisplay(message.body))
  const [body, setBody] = useState(shown.text)
  const picks = useMentionPicks(shown.picks)
  const [error, setError] = useState<string | null>(null)
  const mention = useInlineMention()
  const textarea = useRef<HTMLTextAreaElement>(null)
  const coarse = useCoarsePointer()
  const edit = useEditMessage(queryKey)
  const text = cleanText(body)
  const wire = picks.wire(text)
  const busy = edit.isPending

  useEffect(() => {
    const element = textarea.current
    if (element === null) return
    element.focus()
    element.setSelectionRange(element.value.length, element.value.length)
  }, [])

  function dismiss() {
    if (!busy) onClose()
  }

  function save() {
    if (busy || text === "") return
    if (wire === message.body) {
      onClose()
      return
    }
    setError(null)
    edit.mutate(
      { messageId: message.id, body: wire },
      {
        onSuccess: onClose,
        onError: (failure) => setError(describeEditError(failure)),
      }
    )
  }

  function onKeyDown(event: KeyboardEvent<HTMLFormElement>) {
    if (event.key === "Escape") {
      event.preventDefault()
      event.stopPropagation()
      dismiss()
    } else if (
      (isModEnter(event) && !event.nativeEvent.isComposing) ||
      (event.target === textarea.current && sendsOnEnter(event, coarse))
    ) {
      event.preventDefault()
      save()
    }
  }

  return (
    <form
      aria-label="Edit this comment"
      className="flex flex-col gap-2"
      onSubmit={(event) => {
        event.preventDefault()
        save()
      }}
      onKeyDown={onKeyDown}
    >
      <Textarea
        ref={textarea}
        aria-label="Edit comment"
        rows={2}
        className="field-sizing-content max-h-40 min-h-14 text-[13px]"
        maxLength={maxPromptLength}
        value={body}
        readOnly={busy}
        onChange={(event) => {
          mention.typed(body, event.target.value, event.target.selectionStart)
          setBody(event.target.value)
        }}
      />
      <SendHint action="save" />
      {error === null ? null : (
        <p role="alert" className="text-xs text-destructive">
          {error}
        </p>
      )}
      <div className="flex flex-wrap items-center justify-end gap-2">
        <MentionButton
          agents={noAgents}
          people={people}
          target={target}
          onPick={picks.pick}
          field={textarea}
          value={body}
          onChange={setBody}
          open={mention.open}
          onOpenChange={mention.onOpenChange}
          replaceFrom={mention.replaceFrom}
          disabled={busy}
          className="mr-auto"
        />
        <Button
          type="button"
          variant="ghost"
          size="sm"
          aria-disabled={busy || undefined}
          onClick={dismiss}
        >
          Cancel
        </Button>
        <Button
          type="submit"
          size="sm"
          disabled={text === ""}
          aria-disabled={busy || undefined}
        >
          Save
        </Button>
      </div>
    </form>
  )
}
