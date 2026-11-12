import { useNavigate, useRouteContext } from "@tanstack/react-router"
import { ArrowUpIcon } from "lucide-react"
import { useEffect, useId, useRef, useState } from "react"
import type { FormEvent, KeyboardEvent } from "react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import { useDatasources } from "@/datasources/queries"
import { useCoarsePointer } from "@/lib/coarse-pointer"
import { isEditableTarget } from "@/lib/shortcuts"
import {
  describeSubmitError,
  useAgentInfo,
  useCreateObserveRequest,
  useCreatePending,
} from "@/observe/queries"
import type { ObserveRequest } from "@/observe/queries"
import { StopRequestButton } from "@/observe/request-actions"
import { PromptPicker } from "@/prompts/prompt-picker"
import { maxPromptLength } from "@/prompts/rules"
import { usePromptPicker } from "@/prompts/use-prompt-picker"

import { agentLabel } from "./agent-label"
import { ContextChips, type ContextChip } from "./context-chips"
import { ImportLink } from "./import-link"
import { NoAgentNote } from "./no-agent-note"
import {
  freshCursor,
  stepHistory,
  usePromptHistory,
  type HistoryCursor,
} from "./prompt-history"
import { planSend, type Opens } from "./send-plan"
import {
  availableCommands,
  commandHints,
  commandLabels,
  matchingCommands,
  takeCommand,
  withCommand,
  type SlashCommand,
} from "./slash-commands"

const keyboardHint =
  "Enter sends. Shift+Enter adds a new line. Up arrow recalls an earlier prompt. Type a slash for commands."

const openFailedMessage = "Request created, but it could not be opened."

type Notice = { text: string; invalid: boolean }

type Props = {
  dashboardId?: string
  dashboardTitle?: string
  runningRequestId?: string
  placeholder: string
  rows: number
  autoFocus?: boolean
  disabled?: boolean
  promptPicker?: boolean
  onSent: (request: ObserveRequest, opens: Opens) => void | Promise<void>
}

export function useOpenSent(append?: (request: ObserveRequest) => void) {
  const navigate = useNavigate()
  return async (request: ObserveRequest, opens: Opens) => {
    if (opens === "same" && append !== undefined) {
      append(request)
      return
    }
    await navigate({
      to: opens === "build" ? "/b/$requestId" : "/c/$requestId",
      params: { requestId: request.id },
      viewTransition: opens === "build",
    })
  }
}

export function useSendExample() {
  const create = useCreateObserveRequest()
  const openSent = useOpenSent()

  async function send(prompt: string, dashboardId?: string) {
    try {
      const request = await create.mutateAsync({ prompt, dashboardId })
      await openSent(request, dashboardId === undefined ? "build" : "chat")
    } catch (cause) {
      toast.error(describeSubmitError(cause).message)
    }
  }

  return { send, pending: useCreatePending() }
}

function PickerPopup({
  picker,
}: {
  picker: ReturnType<typeof usePromptPicker>
}) {
  const { principal } = useRouteContext({ from: "/_authed" })
  return <PromptPicker picker={picker} principalId={principal.id} />
}

export function ChatComposer({
  dashboardId,
  dashboardTitle,
  runningRequestId,
  placeholder,
  rows,
  autoFocus = false,
  disabled = false,
  promptPicker = false,
  onSent,
}: Props) {
  const agent = useAgentInfo()
  const datasources = useDatasources()
  const create = useCreateObserveRequest()
  const creating = useCreatePending()
  const coarse = useCoarsePointer()
  const history = usePromptHistory()
  const [prompt, setPrompt] = useState("")
  const [command, setCommand] = useState<SlashCommand | null>(null)
  const [cursor, setCursor] = useState<HistoryCursor>(freshCursor)
  const [announcement, setAnnouncement] = useState("")
  const [submitting, setSubmitting] = useState(false)
  const [notice, setNotice] = useState<Notice | null>(null)
  const field = useRef<HTMLTextAreaElement>(null)
  const inputId = useId()
  const messageId = useId()
  const hintId = useId()
  const commandHintId = useId()

  function placeCaret(at: "start" | "end") {
    requestAnimationFrame(() => {
      const element = field.current
      if (element === null) return
      const position = at === "start" ? 0 : element.value.length
      element.setSelectionRange(position, position)
    })
  }

  function onFill(body: string) {
    setPrompt(body)
    setNotice(null)
    placeCaret("end")
  }

  const picker = usePromptPicker({
    enabled: promptPicker,
    value: prompt,
    onFill,
  })

  const available = availableCommands({
    hasDashboard: dashboardId !== undefined,
    surface: "chat",
    canInvestigate: true,
  })
  const matches = matchingCommands(prompt, available)
  const agentOff = agent.data?.available === false
  const agentPending = agent.isPending
  const busy = submitting || creating || agentPending
  const model = agentLabel(agent.data)
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
        isEditableTarget(event.target)
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

  const describedBy =
    [
      coarse ? null : hintId,
      matches.length > 0 ? commandHintId : null,
      notice !== null ? messageId : null,
    ]
      .filter((id) => id !== null)
      .join(" ") || undefined

  function reject(next: Notice) {
    setNotice(next)
    field.current?.focus()
  }

  function applyText(value: string): string {
    const taken = takeCommand(value, available)
    if (taken === null) return value
    setCommand(taken.command)
    setAnnouncement(`${commandLabels[taken.command]} on`)
    return taken.rest
  }

  function clearCommand() {
    setCommand(null)
    setAnnouncement("")
  }

  function removeCommandChip() {
    clearCommand()
    document.getElementById(inputId)?.focus()
  }

  function recall(direction: "older" | "newer"): boolean {
    const step = stepHistory(
      history.entries,
      cursor,
      direction,
      withCommand(command, prompt)
    )
    if (step === null) return false
    setCursor(step.cursor)
    setCommand(null)
    setPrompt(applyText(step.text))
    setNotice(null)
    placeCaret(direction === "older" ? "start" : "end")
    return true
  }

  async function submit() {
    if (busy || disabled) return
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
    const plan = planSend({ command, text: trimmed, dashboardId })
    if ("error" in plan) {
      reject({ text: plan.error, invalid: true })
      return
    }
    setNotice(null)
    setSubmitting(true)
    try {
      let request: ObserveRequest
      try {
        request = await create.mutateAsync(plan.input)
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
      history.record(withCommand(command, trimmed))
      setPrompt("")
      setCommand(null)
      setCursor(freshCursor)
      setAnnouncement("")
      try {
        await onSent(request, plan.opens)
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
    if (picker.handleKeyDown(event)) return
    const element = event.currentTarget
    const plain =
      !event.shiftKey && !event.altKey && !event.ctrlKey && !event.metaKey
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
    if (event.key === "Backspace" && prompt === "" && command !== null) {
      event.preventDefault()
      clearCommand()
      return
    }
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

  const chips: ContextChip[] = [
    ...(command === null
      ? []
      : [
          {
            id: "command",
            label: commandLabels[command],
            onRemove: removeCommandChip,
          },
        ]),
    ...(dashboardTitle === undefined
      ? []
      : [{ id: "dashboard", label: dashboardTitle }]),
  ]

  return (
    <div className="w-full">
      <div className="relative">
        <form
          onSubmit={onSubmit}
          className="flex w-full flex-col rounded-card border bg-background focus-within:ring-2 focus-within:ring-ring has-aria-invalid:border-destructive has-aria-invalid:ring-2 has-aria-invalid:ring-destructive/20 has-aria-invalid:focus-within:border-ring has-aria-invalid:focus-within:ring-ring"
        >
          <label htmlFor={inputId} className="sr-only">
            Message the agent
          </label>
          {coarse ? null : (
            <p id={hintId} className="sr-only">
              {keyboardHint}
            </p>
          )}
          {chips.length > 0 ? (
            <div className="px-4 pt-3">
              <ContextChips chips={chips} />
            </div>
          ) : null}
          <textarea
            ref={field}
            id={inputId}
            value={prompt}
            rows={rows}
            placeholder={placeholder}
            readOnly={submitting || creating}
            disabled={disabled}
            aria-invalid={notice?.invalid === true}
            aria-describedby={describedBy}
            {...picker.textareaAttributes}
            enterKeyHint={coarse ? "enter" : "send"}
            className="max-h-60 resize-none border-0 bg-transparent px-4 pt-3.5 pb-1.5 text-[15px] leading-[1.5] outline-none placeholder:text-muted-foreground disabled:cursor-not-allowed disabled:opacity-60 narrow:text-base"
            onChange={(event) => {
              const next = applyText(event.target.value)
              setPrompt(next)
              setCursor(freshCursor)
              setNotice(null)
              picker.onValueChange(next)
            }}
            onBlur={picker.dismiss}
            onKeyDown={onKeyDown}
          />
          <div className="flex flex-wrap items-center justify-between gap-2 pt-1.5 pr-2 pb-2 pl-4">
            <div className="flex flex-wrap items-center gap-3.5 text-xs text-muted-foreground">
              {healthy.map((datasource) => (
                <span
                  key={datasource.uid}
                  className="flex items-center gap-1.5"
                >
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
              <ImportLink />
              {runningRequestId !== undefined ? (
                <StopRequestButton requestId={runningRequestId} />
              ) : null}
              <Button
                type="submit"
                size="icon"
                aria-label="Send"
                aria-busy={submitting || creating}
                disabled={submitting || creating || disabled}
                focusableWhenDisabled
                className="size-8"
              >
                <ArrowUpIcon className="size-[15px]" />
              </Button>
            </div>
          </div>
        </form>
        {picker.open ? <PickerPopup picker={picker} /> : null}
      </div>
      <p role="status" className="sr-only">
        {announcement !== "" ? announcement : picker.announcement}
      </p>
      {matches.length > 0 ? (
        <p
          id={commandHintId}
          className="flex flex-wrap gap-x-3 pt-1.5 text-xs text-muted-foreground"
        >
          {matches.map((name) => (
            <span key={name}>
              <span className="font-mono">/{name}</span> {commandHints[name]}
            </span>
          ))}
          <span>Type a space after the command to use it.</span>
        </p>
      ) : null}
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
