import { useNavigate } from "@tanstack/react-router"
import { useEffect, useId, useRef, useState } from "react"
import type { FormEvent, KeyboardEvent } from "react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"
import {
  agentOffMessage,
  describeSubmitError,
  useAgentInfo,
  useCreateObserveRequest,
} from "@/observe/queries"

const maxPromptLength = 4000

function isEditable(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false
  return (
    target.isContentEditable ||
    target instanceof HTMLInputElement ||
    target instanceof HTMLTextAreaElement ||
    target instanceof HTMLSelectElement
  )
}

export function PromptBar() {
  const navigate = useNavigate()
  const agent = useAgentInfo()
  const create = useCreateObserveRequest()
  const [prompt, setPrompt] = useState("")
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const field = useRef<HTMLTextAreaElement>(null)
  const inputId = useId()
  const messageId = useId()

  const agentOff = agent.data?.available === false
  const message = agentOff ? agentOffMessage : error

  useEffect(() => {
    function focusOnSlash(event: globalThis.KeyboardEvent) {
      if (
        event.key !== "/" ||
        event.defaultPrevented ||
        event.isComposing ||
        event.ctrlKey ||
        event.metaKey ||
        event.altKey ||
        isEditable(event.target)
      ) {
        return
      }
      event.preventDefault()
      field.current?.focus()
    }
    window.addEventListener("keydown", focusOnSlash)
    return () => window.removeEventListener("keydown", focusOnSlash)
  }, [])

  async function submit() {
    if (submitting || agentOff) return
    const trimmed = prompt.trim()
    if (trimmed === "") {
      setError("Describe the dashboard you want first.")
      return
    }
    if (trimmed.length > maxPromptLength) {
      setError(`Use at most ${maxPromptLength} characters.`)
      return
    }
    setError(null)
    setSubmitting(true)
    try {
      const request = await create.mutateAsync(trimmed)
      await navigate({
        to: "/observe/$requestId",
        params: { requestId: request.id },
      })
    } catch (cause) {
      const described = describeSubmitError(cause)
      if (described.inline) setError(described.message)
      else toast.error(described.message)
      setSubmitting(false)
    }
  }

  function onSubmit(event: FormEvent) {
    event.preventDefault()
    void submit()
  }

  function onKeyDown(event: KeyboardEvent<HTMLTextAreaElement>) {
    if (
      event.key === "Enter" &&
      !event.shiftKey &&
      !event.nativeEvent.isComposing
    ) {
      event.preventDefault()
      void submit()
    }
  }

  return (
    <form onSubmit={onSubmit} className="mb-8 flex flex-col gap-2">
      <label htmlFor={inputId} className="text-sm font-medium">
        Describe the dashboard you want
      </label>
      <Textarea
        ref={field}
        id={inputId}
        value={prompt}
        rows={2}
        className="max-h-60 resize-none"
        placeholder="checkout health by pod, last 6h"
        disabled={agentOff}
        readOnly={submitting}
        aria-invalid={error !== null}
        aria-describedby={message === null ? undefined : messageId}
        onChange={(event) => {
          setPrompt(event.target.value)
          setError(null)
        }}
        onKeyDown={onKeyDown}
      />
      <div className="flex items-start justify-between gap-4">
        <div className="flex min-h-8 min-w-0 items-center text-sm">
          {message === null ? (
            <p className="text-muted-foreground max-sm:hidden">
              Press Enter to send and Shift+Enter for a new line.
            </p>
          ) : (
            <p
              id={messageId}
              role={agentOff ? undefined : "alert"}
              className={
                agentOff ? "text-muted-foreground" : "text-destructive"
              }
            >
              {message}
            </p>
          )}
        </div>
        <Button
          type="submit"
          disabled={agentOff || submitting}
          aria-busy={submitting}
        >
          Observe
        </Button>
      </div>
    </form>
  )
}
