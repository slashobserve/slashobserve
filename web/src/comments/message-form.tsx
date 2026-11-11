import { useRef, useState, type KeyboardEvent } from "react"

import { ApiError } from "@/api/errors"
import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"
import { isModEnter } from "@/lib/shortcuts"
import { sentence } from "@/observe/queries"
import { maxPromptLength } from "@/revisions/queries"

import { MentionButton } from "./mention-button"
import type { MentionPerson } from "./mentions"

function describeSendError(error: unknown): string {
  if (error instanceof ApiError && error.status < 500) {
    return sentence(error.message)
  }
  return "The comment could not be sent. Try again."
}

export function MessageForm({
  label,
  submitLabel,
  placeholder,
  people,
  onSubmit,
  onCancel,
  autoFocus,
}: {
  label: string
  submitLabel: string
  placeholder?: string
  people: readonly MentionPerson[]
  onSubmit: (body: string) => Promise<unknown>
  onCancel?: () => void
  autoFocus?: boolean
}) {
  const [body, setBody] = useState("")
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const textarea = useRef<HTMLTextAreaElement>(null)
  const text = body.trim()

  async function submit() {
    if (busy || text === "") return
    setBusy(true)
    setError(null)
    try {
      await onSubmit(text)
      setBody("")
    } catch (failure) {
      setError(describeSendError(failure))
    } finally {
      setBusy(false)
    }
  }

  function onKeyDown(event: KeyboardEvent<HTMLFormElement>) {
    if (event.key === "Escape" && onCancel !== undefined && !busy) {
      event.preventDefault()
      event.stopPropagation()
      onCancel()
    } else if (isModEnter(event) && !event.nativeEvent.isComposing) {
      event.preventDefault()
      void submit()
    }
  }

  return (
    <form
      aria-label={label}
      className="flex flex-col gap-2"
      onSubmit={(event) => {
        event.preventDefault()
        void submit()
      }}
      onKeyDown={onKeyDown}
    >
      <Textarea
        ref={textarea}
        aria-label={label}
        autoFocus={autoFocus}
        rows={2}
        placeholder={placeholder}
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
          people={people}
          field={textarea}
          value={body}
          onChange={setBody}
          disabled={busy}
          className="mr-auto"
        />
        {onCancel === undefined ? null : (
          <Button
            type="button"
            variant="ghost"
            size="sm"
            aria-disabled={busy || undefined}
            onClick={() => {
              if (!busy) onCancel()
            }}
          >
            Cancel
          </Button>
        )}
        <Button
          type="submit"
          size="sm"
          disabled={text === ""}
          aria-disabled={busy || undefined}
        >
          {submitLabel}
        </Button>
      </div>
    </form>
  )
}
