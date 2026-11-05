import {
  useEffect,
  useId,
  useImperativeHandle,
  useRef,
  useState,
  type FormEvent,
  type KeyboardEvent,
  type ReactNode,
  type Ref,
} from "react"

import { ApiError } from "@/api/errors"
import { agentLabel } from "@/chat/agent-label"
import { ContextChips, type ContextChip } from "@/chat/context-chips"
import {
  freshCursor,
  stepHistory,
  usePromptHistory,
  type HistoryCursor,
} from "@/chat/prompt-history"
import {
  availableCommands,
  commandLabels,
  takeCommand,
  withCommand,
  type SlashCommand,
} from "@/chat/slash-commands"
import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"
import { useAddComment } from "@/comments/queries"
import { liveRegionText } from "@/lib/live-region"
import { isModEnter } from "@/lib/shortcuts"
import { cn } from "@/lib/utils"
import { sentence, useAgentInfo } from "@/observe/queries"
import { StopRequestButton } from "@/observe/request-actions"
import {
  describeRevisionError,
  isAgentUnavailable,
  maxPromptLength,
  useRequestRevision,
} from "@/revisions/queries"

import type { AgentGate } from "./agent-gate"

export type Target = { kind: "dashboard" } | { kind: "panel"; panelId: string }

type Props = {
  dashboardId: string
  latestVersion: number
  panels: readonly { id: string; title: string }[]
  target: Target
  onTargetChange: (target: Target) => void
  draft: string
  onDraftChange: (text: string) => void
  gate: AgentGate
  placeholder: string
  textareaRef?: Ref<HTMLTextAreaElement>
  notice?: ReactNode
  runningRequestId?: string
}

const counterThreshold = 3500
const commands = availableCommands({
  hasDashboard: true,
  surface: "activity",
})
const panelGoneText = "That panel no longer exists. Pick another target."
const agentGoneText =
  "The agent is no longer available. Your text is still here."

function plural(count: number): string {
  return count === 1 ? "character" : "characters"
}

function commentFailureText(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 422 && error.code === "unknown_panel") {
      return panelGoneText
    }
    return error.status >= 500
      ? "The comment could not be posted right now. Try again."
      : sentence(error.message)
  }
  return "The comment could not be posted. Check your connection and try again."
}

function requestFailureText(error: unknown): string {
  if (
    error instanceof ApiError &&
    error.status === 422 &&
    error.code === "unknown_panel"
  ) {
    return panelGoneText
  }
  if (isAgentUnavailable(error)) return agentGoneText
  return describeRevisionError(error)
}

export function ActivityComposer({
  dashboardId,
  latestVersion,
  panels,
  target,
  onTargetChange,
  draft,
  onDraftChange,
  gate,
  placeholder,
  textareaRef,
  notice,
  runningRequestId,
}: Props) {
  const counterId = useId()
  const textareaId = useId()
  const errorId = useId()
  const addComment = useAddComment(dashboardId)
  const requestRevision = useRequestRevision(dashboardId)
  const field = useRef<HTMLTextAreaElement>(null)
  const [failure, setFailure] = useState<string | null>(null)
  const [note, setNote] = useState({ text: "", count: 0 })
  const [command, setCommand] = useState<SlashCommand | null>(null)
  const [cursor, setCursor] = useState<HistoryCursor>(freshCursor)
  const history = usePromptHistory()
  const agent = useAgentInfo()
  const model = agentLabel(agent.data)

  useImperativeHandle(
    textareaRef,
    () => field.current as HTMLTextAreaElement,
    []
  )

  const targetMissing =
    target.kind === "panel" &&
    !panels.some((panel) => panel.id === target.panelId)
  const targetGone = targetMissing && panels.length > 0
  useEffect(() => {
    if (targetGone) onTargetChange({ kind: "dashboard" })
  }, [targetGone, onTargetChange])

  const panelId =
    target.kind === "panel" && !targetMissing ? target.panelId : null
  const trimmed = draft.trim()
  const length = [...trimmed].length
  const over = length > maxPromptLength
  const showCounter = length >= counterThreshold
  const pendingComment = addComment.isPending
  const pendingRequest = requestRevision.isPending
  const pending = pendingComment || pendingRequest
  const waitingForPanels = target.kind === "panel" && panels.length === 0
  const cannotSubmit = trimmed === "" || over || waitingForPanels

  function announce(text: string) {
    setNote((current) => ({ text, count: current.count + 1 }))
  }

  function succeeded(text: string) {
    onDraftChange("")
    setCommand(null)
    setCursor(freshCursor)
    announce(text)
    field.current?.focus()
  }

  function applyText(value: string): string {
    const taken = takeCommand(value, commands)
    if (taken === null) return value
    setCommand(taken.command)
    announce(`${commandLabels[taken.command]} on`)
    return taken.rest
  }

  function recall(direction: "older" | "newer"): boolean {
    const step = stepHistory(
      history.entries,
      cursor,
      direction,
      withCommand(command, draft)
    )
    if (step === null) return false
    setCursor(step.cursor)
    setCommand(null)
    onDraftChange(applyText(step.text))
    requestAnimationFrame(() => {
      const element = field.current
      if (element === null) return
      const at = direction === "older" ? 0 : element.value.length
      element.setSelectionRange(at, at)
    })
    return true
  }

  function removeCommandChip() {
    setCommand(null)
    document.getElementById(textareaId)?.focus()
  }

  function failed(text: string) {
    setFailure(text)
    field.current?.focus()
  }

  function addCommentNow() {
    if (cannotSubmit || pending) return
    setFailure(null)
    addComment.mutate(
      { body: trimmed, version: latestVersion, panelId },
      {
        onSuccess: () => succeeded("Comment added"),
        onError: (error) => failed(commentFailureText(error)),
      }
    )
  }

  function sendToAgent() {
    if (cannotSubmit || pending) return
    setFailure(null)
    requestRevision.mutate(
      { prompt: trimmed, baseVersion: latestVersion, panelId },
      {
        onSuccess: () => {
          history.record(withCommand(command, trimmed))
          succeeded("Sent to the agent")
        },
        onError: (error) => failed(requestFailureText(error)),
      }
    )
  }

  function onSubmit(event: FormEvent) {
    event.preventDefault()
    addCommentNow()
  }

  function onKeyDown(event: KeyboardEvent<HTMLTextAreaElement>) {
    if (isModEnter(event) && !event.nativeEvent.isComposing) {
      event.preventDefault()
      addCommentNow()
      return
    }
    const element = event.currentTarget
    const plain =
      !event.shiftKey && !event.altKey && !event.ctrlKey && !event.metaKey
    const browsing = cursor.index !== -1
    if (
      plain &&
      event.key === "ArrowUp" &&
      (draft === "" || browsing) &&
      element.selectionStart === 0 &&
      element.selectionEnd === 0
    ) {
      if (recall("older")) event.preventDefault()
      return
    }
    if (
      plain &&
      event.key === "ArrowDown" &&
      browsing &&
      element.selectionStart === element.value.length
    ) {
      if (recall("newer")) event.preventDefault()
      return
    }
    if (event.key === "Backspace" && draft === "" && command !== null) {
      event.preventDefault()
      setCommand(null)
    }
  }

  const chips: ContextChip[] =
    command === null
      ? []
      : [
          {
            id: "command",
            label: commandLabels[command],
            onRemove: removeCommandChip,
          },
        ]

  const describedBy =
    [showCounter ? counterId : null, failure ? errorId : null]
      .filter(Boolean)
      .join(" ") || undefined

  return (
    <form
      aria-label="Add to the activity"
      onSubmit={onSubmit}
      className="flex flex-col gap-2 border-t px-4 pt-3 pb-3.5"
    >
      {notice}
      <label className="flex items-center gap-1.5 text-xs text-muted-foreground">
        On
        <select
          value={panelId ?? ""}
          onChange={(event) =>
            onTargetChange(
              event.target.value === ""
                ? { kind: "dashboard" }
                : { kind: "panel", panelId: event.target.value }
            )
          }
          className="h-[26px] max-w-[240px] rounded-[5px] border bg-background px-1.5 text-xs text-foreground"
        >
          <option value="">The whole dashboard</option>
          {panels.map((panel) => (
            <option key={panel.id} value={panel.id}>
              {panel.title === "" ? "Untitled panel" : panel.title}
            </option>
          ))}
        </select>
      </label>
      <ContextChips chips={chips} />
      <Textarea
        ref={field}
        id={textareaId}
        aria-label="Comment or request"
        aria-invalid={over || undefined}
        aria-describedby={describedBy}
        placeholder={placeholder}
        rows={3}
        value={draft}
        readOnly={pending}
        className="field-sizing-fixed max-h-64 min-h-16 resize-y px-3 py-2.5 text-sm"
        onChange={(event) => {
          onDraftChange(applyText(event.target.value))
          setCursor(freshCursor)
          if (failure) setFailure(null)
        }}
        onKeyDown={onKeyDown}
      />
      {showCounter ? (
        <span
          id={counterId}
          className={cn(
            "text-xs text-muted-foreground tabular-nums",
            over && "text-status-critical"
          )}
        >
          {over
            ? `${length - maxPromptLength} ${plural(length - maxPromptLength)} over`
            : `${maxPromptLength - length} ${plural(maxPromptLength - length)} left`}
        </span>
      ) : null}
      <div className="flex flex-wrap items-center justify-end gap-2">
        {model !== null ? (
          <span className="mr-auto text-xs text-muted-foreground">{model}</span>
        ) : null}
        <Button
          type="submit"
          variant="outline"
          disabled={cannotSubmit}
          aria-disabled={pending || undefined}
          aria-busy={pendingComment || undefined}
        >
          Add comment
        </Button>
        {runningRequestId !== undefined ? (
          <StopRequestButton requestId={runningRequestId} />
        ) : null}
        {gate.kind === "allowed" ? (
          <Button
            type="button"
            disabled={cannotSubmit}
            aria-disabled={pending || undefined}
            aria-busy={pendingRequest || undefined}
            onClick={sendToAgent}
          >
            Send to agent
          </Button>
        ) : null}
      </div>
      {gate.kind === "blocked" ? (
        <p className="text-xs text-muted-foreground">{gate.reason}</p>
      ) : null}
      {failure ? (
        <p id={errorId} role="alert" className="text-xs text-status-critical">
          {failure}
        </p>
      ) : null}
      <span role="status" className="sr-only">
        {liveRegionText(note.text, note.count)}
      </span>
    </form>
  )
}
