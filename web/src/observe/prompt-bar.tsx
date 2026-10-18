import { useNavigate } from "@tanstack/react-router"
import { useEffect, useId, useRef, useState, useSyncExternalStore } from "react"
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

type Notice = { text: string; invalid: boolean }

function isEditable(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false
  return (
    target.isContentEditable ||
    target instanceof HTMLInputElement ||
    target instanceof HTMLTextAreaElement ||
    target instanceof HTMLSelectElement
  )
}

const coarsePointer = "(pointer: coarse)"

function subscribeCoarsePointer(onChange: () => void) {
  const media = window.matchMedia(coarsePointer)
  media.addEventListener("change", onChange)
  return () => media.removeEventListener("change", onChange)
}

function useCoarsePointer(): boolean {
  return useSyncExternalStore(
    subscribeCoarsePointer,
    () => window.matchMedia(coarsePointer).matches,
    () => false
  )
}

export function PromptBar() {
  const navigate = useNavigate()
  const agent = useAgentInfo()
  const create = useCreateObserveRequest()
  const coarse = useCoarsePointer()
  const [prompt, setPrompt] = useState("")
  const [submitting, setSubmitting] = useState(false)
  const [notice, setNotice] = useState<Notice | null>(null)
  const field = useRef<HTMLTextAreaElement>(null)
  const inputId = useId()
  const hintId = useId()
  const messageId = useId()

  const agentOff = agent.data?.available === false
  const message = agentOff ? agentOffMessage : (notice?.text ?? null)

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
      const element = field.current
      if (element === null || element.disabled) return
      event.preventDefault()
      element.focus()
    }
    window.addEventListener("keydown", focusOnSlash)
    return () => window.removeEventListener("keydown", focusOnSlash)
  }, [])

  function reject(next: Notice) {
    setNotice(next)
    field.current?.focus()
  }

  async function submit() {
    if (submitting || agentOff) return
    const trimmed = prompt.trim()
    if (trimmed === "") {
      reject({ text: "Describe the dashboard you want first.", invalid: true })
      return
    }
    if ([...trimmed].length > maxPromptLength) {
      reject({
        text: `Use at most ${maxPromptLength} characters.`,
        invalid: true,
      })
      return
    }
    setNotice(null)
    setSubmitting(true)
    try {
      const request = await create.mutateAsync({ prompt: trimmed })
      await navigate({
        to: "/observe/$requestId",
        params: { requestId: request.id },
      })
    } catch (cause) {
      const described = describeSubmitError(cause)
      setSubmitting(false)
      if (described.inline) {
        reject({ text: described.message, invalid: described.invalid })
      } else {
        toast.error(described.message)
        field.current?.focus()
      }
    }
  }

  function onSubmit(event: FormEvent) {
    event.preventDefault()
    void submit()
  }

  function onKeyDown(event: KeyboardEvent<HTMLTextAreaElement>) {
    if (
      !coarse &&
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
        aria-invalid={!agentOff && notice?.invalid === true}
        enterKeyHint={coarse ? "enter" : "send"}
        aria-describedby={
          message !== null ? messageId : coarse ? undefined : hintId
        }
        onChange={(event) => {
          setPrompt(event.target.value)
          setNotice(null)
        }}
        onKeyDown={onKeyDown}
      />
      <div className="flex items-start justify-between gap-4">
        <div className="flex min-h-8 min-w-0 items-center text-sm">
          {message === null ? (
            coarse ? null : (
              <p id={hintId} className="text-balance text-muted-foreground">
                Enter sends. Shift+Enter adds a new line.
              </p>
            )
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
