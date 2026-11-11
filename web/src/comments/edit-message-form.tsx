import { useEffect, useRef, useState, type KeyboardEvent } from "react"
import type { QueryKey } from "@tanstack/react-query"

import { ApiError } from "@/api/errors"
import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"
import { isModEnter } from "@/lib/shortcuts"
import { sentence } from "@/observe/queries"
import { maxPromptLength } from "@/revisions/queries"

import { MentionButton } from "./mention-button"
import { mentionOptions, type MentionPerson } from "./mentions"
import { useEditMessage } from "./queries"

function describeEditError(error: unknown): string {
  if (error instanceof ApiError && error.status < 500) {
    return sentence(error.message)
  }
  return "The comment could not be saved. Try again."
}

export function EditMessageForm({
  message,
  people,
  queryKey,
  onClose,
}: {
  message: { id: string; body: string }
  people: readonly MentionPerson[]
  queryKey: QueryKey
  onClose: () => void
}) {
  const [body, setBody] = useState(message.body)
  const [error, setError] = useState<string | null>(null)
  const [mentioning, setMentioning] = useState(false)
  const textarea = useRef<HTMLTextAreaElement>(null)
  const edit = useEditMessage(queryKey)
  const text = body.trim()
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
    if (text === message.body) {
      onClose()
      return
    }
    setError(null)
    edit.mutate(
      { messageId: message.id, body: text },
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
    } else if (isModEnter(event) && !event.nativeEvent.isComposing) {
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
        onChange={(event) => setBody(event.target.value)}
      />
      {error === null ? null : (
        <p role="alert" className="text-xs text-destructive">
          {error}
        </p>
      )}
      <div className="flex flex-wrap items-center justify-end gap-2">
        <MentionButton
          options={mentionOptions(people, [])}
          field={textarea}
          value={body}
          onChange={setBody}
          open={mentioning}
          onOpenChange={setMentioning}
          replaceFrom={null}
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
