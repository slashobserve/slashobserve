import { useNavigate } from "@tanstack/react-router"
import { ArrowUpIcon } from "lucide-react"
import { useEffect, useId, useRef, useState } from "react"
import type { FormEvent, KeyboardEvent, ReactNode } from "react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import { useDatasources } from "@/datasources/queries"
import { useCoarsePointer } from "@/lib/coarse-pointer"
import {
  describeSubmitError,
  useAgentInfo,
  useCreateObserveRequest,
} from "@/observe/queries"
import type { ObserveRequest } from "@/observe/queries"

import { NoAgentNote } from "./no-agent-note"

const maxPromptLength = 4000

const openFailedMessage = "Request created, but it could not be opened."

type Notice = { text: string; invalid: boolean }

type Props = {
  dashboardId?: string
  placeholder: string
  rows: number
  autoFocus?: boolean
  onSent: (request: ObserveRequest) => void | Promise<void>
  secondaryAction?: ReactNode
}

function isEditable(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false
  return (
    target.isContentEditable ||
    target instanceof HTMLInputElement ||
    target instanceof HTMLTextAreaElement ||
    target instanceof HTMLSelectElement
  )
}

export function useSendExample() {
  const navigate = useNavigate()
  const create = useCreateObserveRequest()

  async function send(prompt: string) {
    try {
      const request = await create.mutateAsync({ prompt })
      await navigate({
        to: "/observe/$requestId",
        params: { requestId: request.id },
      })
    } catch (cause) {
      toast.error(describeSubmitError(cause).message)
    }
  }

  return { send, pending: create.isPending }
}

export function ChatComposer({
  dashboardId,
  placeholder,
  rows,
  autoFocus = false,
  onSent,
  secondaryAction,
}: Props) {
  const agent = useAgentInfo()
  const datasources = useDatasources()
  const create = useCreateObserveRequest()
  const coarse = useCoarsePointer()
  const [prompt, setPrompt] = useState("")
  const [submitting, setSubmitting] = useState(false)
  const [notice, setNotice] = useState<Notice | null>(null)
  const field = useRef<HTMLTextAreaElement>(null)
  const inputId = useId()
  const messageId = useId()

  const agentOff = agent.data?.available === false
  const model = agent.data?.model ?? null
  const healthy = (datasources.data ?? []).filter((d) => d.healthy)

  useEffect(() => {
    if (autoFocus) field.current?.focus()
  }, [autoFocus, agentOff])

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

  if (agentOff) return <NoAgentNote />

  function reject(next: Notice) {
    setNotice(next)
    field.current?.focus()
  }

  async function submit() {
    if (submitting) return
    const trimmed = prompt.trim()
    if (trimmed === "") {
      reject({
        text:
          dashboardId === undefined
            ? "Describe the dashboard you want first."
            : "Describe the change you want first.",
        invalid: true,
      })
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
      let request: ObserveRequest
      try {
        request = await create.mutateAsync({ prompt: trimmed, dashboardId })
      } catch (cause) {
        const described = describeSubmitError(cause)
        if (described.inline) {
          reject({ text: described.message, invalid: described.invalid })
        } else {
          toast.error(described.message)
          field.current?.focus()
        }
        return
      }
      setPrompt("")
      try {
        await onSent(request)
      } catch {
        toast.error(openFailedMessage)
      }
    } finally {
      setSubmitting(false)
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
    <div className="w-full">
      <form
        onSubmit={onSubmit}
        className="flex w-full flex-col rounded-card border bg-background focus-within:ring-2 focus-within:ring-ring has-aria-invalid:border-destructive"
      >
        <label htmlFor={inputId} className="sr-only">
          Message the agent
        </label>
        <textarea
          ref={field}
          id={inputId}
          value={prompt}
          rows={rows}
          placeholder={placeholder}
          readOnly={submitting}
          aria-invalid={notice?.invalid === true}
          aria-describedby={notice !== null ? messageId : undefined}
          enterKeyHint={coarse ? "enter" : "send"}
          className="max-h-60 resize-none border-0 bg-transparent px-4 pt-3.5 pb-1.5 text-[15px] leading-[1.5] outline-none placeholder:text-muted-foreground narrow:text-base"
          onChange={(event) => {
            setPrompt(event.target.value)
            setNotice(null)
          }}
          onKeyDown={onKeyDown}
        />
        <div className="flex flex-wrap items-center justify-between gap-2 pt-1.5 pr-2 pb-2 pl-4">
          <div className="flex flex-wrap items-center gap-3.5 text-xs text-muted-foreground">
            {healthy.map((datasource) => (
              <span key={datasource.uid} className="flex items-center gap-1.5">
                <span
                  aria-hidden="true"
                  className="size-[7px] flex-none rounded-full bg-status-ok"
                />
                {datasource.uid}
                <span className="sr-only">, healthy</span>
              </span>
            ))}
            {model !== null ? <span>{model}</span> : null}
          </div>
          <div className="flex items-center gap-1.5">
            {secondaryAction}
            <Button
              type="submit"
              size="icon"
              aria-label="Send"
              aria-busy={submitting}
              disabled={submitting}
              focusableWhenDisabled
              className="size-8"
            >
              <ArrowUpIcon className="size-[15px]" />
            </Button>
          </div>
        </div>
      </form>
      <div className="text-[13px]">
        {notice !== null ? (
          <p id={messageId} role="alert" className="pt-1.5 text-destructive">
            {notice.text}
          </p>
        ) : null}
      </div>
    </div>
  )
}
