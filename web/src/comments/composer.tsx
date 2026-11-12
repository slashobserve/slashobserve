import {
  useRef,
  useState,
  type ChangeEvent,
  type KeyboardEvent,
  type ReactNode,
} from "react"
import { ArrowUpIcon } from "lucide-react"

import { ApiError } from "@/api/errors"
import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"
import { isModEnter } from "@/lib/shortcuts"
import { sentence } from "@/observe/queries"
import { maxPromptLength } from "@/revisions/queries"

import { MentionButton } from "./mention-button"
import {
  mentionOptions,
  mentionTriggerAt,
  type MentionAgent,
  type MentionPerson,
} from "./mentions"

type ComposerProps = {
  label: string
  placeholder: string
  hint?: ReactNode
  people: readonly MentionPerson[]
  agents?: readonly MentionAgent[]
  onSubmit: (body: string) => Promise<unknown>
  onCancel?: () => void
  autoFocus?: boolean
  disabledReason?: string | null
  defaultValue?: string
}

function describeSendError(error: unknown): string {
  if (error instanceof ApiError && error.status < 500) {
    return sentence(error.message)
  }
  return "The comment could not be sent. Try again."
}

export function Composer({
  label,
  placeholder,
  hint,
  people,
  agents = [],
  onSubmit,
  onCancel,
  autoFocus,
  disabledReason,
  defaultValue = "",
}: ComposerProps) {
  const [body, setBody] = useState(defaultValue)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [mentioning, setMentioning] = useState(false)
  const [replaceFrom, setReplaceFrom] = useState<number | null>(null)
  const textarea = useRef<HTMLTextAreaElement>(null)
  const text = body.trim()
  const unavailable = typeof disabledReason === "string"

  async function submit() {
    if (busy || unavailable || text === "") return
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

  function onChange(event: ChangeEvent<HTMLTextAreaElement>) {
    const { value, selectionStart } = event.target
    const typed = value.length > body.length
    setBody(value)
    const trigger = typed ? mentionTriggerAt(value, selectionStart) : null
    if (trigger === null) return
    setReplaceFrom(trigger)
    setMentioning(true)
  }

  return (
    <form
      aria-label={label}
      className="flex flex-col rounded-card border border-border bg-card focus-within:ring-2 focus-within:ring-ring"
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
        className="field-sizing-content max-h-40 min-h-0 rounded-none border-0 bg-transparent pt-2.5 pr-3 pb-1 pl-3 text-[13px] focus-visible:border-0 focus-visible:ring-0"
        maxLength={maxPromptLength}
        value={body}
        disabled={unavailable}
        readOnly={busy}
        onChange={onChange}
      />
      {error === null ? null : (
        <p role="alert" className="px-3 text-xs text-destructive">
          {error}
        </p>
      )}
      <div className="flex items-center gap-1 pt-1 pr-1.5 pb-1.5 pl-3">
        <div className="mr-auto min-w-0 text-xs text-muted-foreground">
          {unavailable ? disabledReason : hint}
        </div>
        <MentionButton
          options={mentionOptions(people, agents)}
          field={textarea}
          value={body}
          onChange={setBody}
          open={mentioning}
          onOpenChange={(next) => {
            setMentioning(next)
            if (!next) setReplaceFrom(null)
          }}
          replaceFrom={replaceFrom}
          disabled={busy || unavailable}
        />
        <Button
          type="submit"
          size="icon-sm"
          aria-label="Send"
          title="Send"
          disabled={text === "" || unavailable}
          aria-disabled={busy || undefined}
        >
          <ArrowUpIcon aria-hidden="true" />
        </Button>
      </div>
    </form>
  )
}
