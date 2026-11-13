import {
  useId,
  useState,
  type FocusEvent,
  type KeyboardEvent,
  type Ref,
} from "react"
import { LoaderCircleIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"
import {
  freshCursor,
  stepHistory,
  usePromptHistory,
  type HistoryCursor,
} from "@/chat/prompt-history"

import type { DraftSignal } from "./draft-query"

const recentShown = 5

export function AskField({
  signal,
  value,
  onChange,
  onAsk,
  pending,
  onCancel,
  placeholder,
  ref,
}: {
  signal: DraftSignal
  value: string
  onChange: (value: string) => void
  onAsk: (question: string) => void
  pending: boolean
  onCancel: () => void
  placeholder: string
  ref?: Ref<HTMLTextAreaElement>
}) {
  const fieldId = useId()
  const recentId = useId()
  const history = usePromptHistory(`slashobserve-ask-history-${signal}`)
  const [cursor, setCursor] = useState<HistoryCursor>(freshCursor)
  const [focused, setFocused] = useState(false)
  const recent = history.entries.slice(0, recentShown)
  const recentOpen = focused && value.trim() === "" && recent.length > 0

  function ask() {
    const question = value.trim()
    if (question === "") return
    history.record(question)
    setCursor(freshCursor)
    onAsk(question)
  }

  function recall(direction: "older" | "newer"): boolean {
    const step = stepHistory(history.entries, cursor, direction, value)
    if (step === null) return false
    setCursor(step.cursor)
    onChange(step.text)
    return true
  }

  function onKeyDown(event: KeyboardEvent<HTMLTextAreaElement>) {
    const element = event.currentTarget
    const plain =
      !event.shiftKey && !event.altKey && !event.ctrlKey && !event.metaKey
    if (event.key === "Escape" && pending) {
      event.preventDefault()
      onCancel()
      return
    }
    if (
      plain &&
      event.key === "ArrowUp" &&
      element.selectionStart === 0 &&
      element.selectionEnd === 0
    ) {
      if (recall("older")) event.preventDefault()
      return
    }
    if (
      plain &&
      event.key === "ArrowDown" &&
      cursor.index !== -1 &&
      element.selectionStart === element.value.length
    ) {
      if (recall("newer")) event.preventDefault()
      return
    }
    if (
      event.key === "Enter" &&
      !event.shiftKey &&
      !event.nativeEvent.isComposing
    ) {
      event.preventDefault()
      ask()
    }
  }

  function onBlur(event: FocusEvent<HTMLDivElement>) {
    if (!event.currentTarget.contains(event.relatedTarget)) setFocused(false)
  }

  return (
    <div
      className="flex flex-col gap-2"
      onFocus={() => setFocused(true)}
      onBlur={onBlur}
    >
      <label htmlFor={fieldId} className="sr-only">
        Question
      </label>
      <div className="flex items-start gap-2">
        <Textarea
          ref={ref}
          id={fieldId}
          rows={1}
          value={value}
          placeholder={placeholder}
          aria-controls={recentOpen ? recentId : undefined}
          onChange={(event) => {
            onChange(event.target.value)
            setCursor(freshCursor)
          }}
          onKeyDown={onKeyDown}
          className="max-h-[4.75rem] min-h-8 flex-1 resize-none bg-background narrow:max-h-24 narrow:min-h-11"
        />
        <Button
          type="button"
          size="sm"
          className="mt-0.5 narrow:mt-0 narrow:h-11"
          onClick={ask}
          aria-busy={pending}
        >
          {pending ? (
            <LoaderCircleIcon
              aria-hidden="true"
              className="motion-safe:animate-spin"
            />
          ) : null}
          {pending ? "Writing query" : "Ask"}
        </Button>
      </div>
      {recentOpen ? (
        <div
          id={recentId}
          role="group"
          aria-label="Recent"
          className="flex flex-col items-start gap-0.5"
        >
          <p className="text-xs text-muted-foreground">Recent</p>
          {recent.map((question) => (
            <Button
              key={question}
              type="button"
              variant="ghost"
              size="xs"
              className="max-w-full justify-start truncate font-normal"
              onClick={() => onChange(question)}
            >
              {question}
            </Button>
          ))}
        </div>
      ) : null}
    </div>
  )
}
