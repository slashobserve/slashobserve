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

import { useQueryClient } from "@tanstack/react-query"

import { ApiError } from "@/api/errors"
import { dashboardKeys } from "@/api/keys"
import { agentLabel } from "@/chat/agent-label"
import { ContextChips, type ContextChip } from "@/chat/context-chips"
import {
  freshCursor,
  stepHistory,
  usePromptHistory,
  type HistoryCursor,
} from "@/chat/prompt-history"
import { planSend } from "@/chat/send-plan"
import {
  availableCommands,
  commandLabels,
  takeCommand,
  withCommand,
} from "@/chat/slash-commands"
import { Button } from "@/components/ui/button"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Textarea } from "@/components/ui/textarea"
import { MentionButton } from "@/comments/mention-button"
import { useAddComment } from "@/comments/queries"
import { liveRegionText } from "@/lib/live-region"
import { describeRange } from "@/lib/time"
import { isModEnter } from "@/lib/shortcuts"
import { cn } from "@/lib/utils"
import {
  describeSubmitError,
  sentence,
  useAgentInfo,
  useCreateObserveRequest,
} from "@/observe/queries"
import { StopRequestButton } from "@/observe/request-actions"
import {
  describeRevisionError,
  isAgentUnavailable,
  maxPromptLength,
  useRequestRevision,
} from "@/revisions/queries"

import { useActivityStore } from "./activity-store"
import type { AgentGate } from "./agent-gate"

export type Target = { kind: "dashboard" } | { kind: "panel"; panelId: string }

export const noPanelValue = "*"

export function panelItems(
  noPanelLabel: string,
  panels: readonly { id: string; title: string }[]
): { value: string; label: string }[] {
  return [
    { value: noPanelValue, label: noPanelLabel },
    ...panels.map((panel) => ({
      value: panel.id,
      label: panel.title === "" ? "Untitled panel" : panel.title,
    })),
  ]
}

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
  range: { from: string; to: string }
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

function investigateFailureText(error: unknown): string {
  if (
    error instanceof ApiError &&
    error.status === 422 &&
    error.code === "unknown_panel"
  ) {
    return panelGoneText
  }
  if (isAgentUnavailable(error)) return agentGoneText
  return describeSubmitError(error).message
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
  range,
}: Props) {
  const counterId = useId()
  const textareaId = useId()
  const errorId = useId()
  const targetId = useId()
  const addComment = useAddComment(dashboardId)
  const requestRevision = useRequestRevision(dashboardId)
  const createRequest = useCreateObserveRequest()
  const queryClient = useQueryClient()
  const agentKind = useActivityStore((state) => state.agentKind)
  const setAgentKind = useActivityStore((state) => state.setAgentKind)
  const investigating = agentKind === "investigate"
  const [rangeOn, setRangeOn] = useState(true)
  const [rangeKind, setRangeKind] = useState(agentKind)
  if (rangeKind !== agentKind) {
    setRangeKind(agentKind)
    setRangeOn(true)
  }
  const field = useRef<HTMLTextAreaElement>(null)
  const [failure, setFailure] = useState<string | null>(null)
  const [note, setNote] = useState({ text: "", count: 0 })
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
  const pendingRequest = requestRevision.isPending || createRequest.isPending
  const pending = pendingComment || pendingRequest
  const waitingForPanels = target.kind === "panel" && panels.length === 0
  const cannotSubmit = trimmed === "" || over || waitingForPanels

  function announce(text: string) {
    setNote((current) => ({ text, count: current.count + 1 }))
  }

  function succeeded(text: string) {
    onDraftChange("")
    setAgentKind("revise")
    setCursor(freshCursor)
    announce(text)
    field.current?.focus()
  }

  function applyText(value: string): string {
    const taken = takeCommand(value, commands)
    if (taken === null) return value
    setAgentKind(taken.command === "investigate" ? "investigate" : "revise")
    announce(`${commandLabels[taken.command]} on`)
    return taken.rest
  }

  function recall(direction: "older" | "newer"): boolean {
    const step = stepHistory(
      history.entries,
      cursor,
      direction,
      withCommand(investigating ? "investigate" : null, draft)
    )
    if (step === null) return false
    setCursor(step.cursor)
    setAgentKind("revise")
    onDraftChange(applyText(step.text))
    requestAnimationFrame(() => {
      const element = field.current
      if (element === null) return
      const at = direction === "older" ? 0 : element.value.length
      element.setSelectionRange(at, at)
    })
    return true
  }

  function focusField() {
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
    if (investigating) {
      investigate()
      return
    }
    requestRevision.mutate(
      { prompt: trimmed, baseVersion: latestVersion, panelId },
      {
        onSuccess: () => {
          history.record(trimmed)
          succeeded("Sent to the agent")
        },
        onError: (error) => failed(requestFailureText(error)),
      }
    )
  }

  function investigate() {
    const plan = planSend({
      command: "investigate",
      text: trimmed,
      dashboardId,
      panelId: panelId ?? undefined,
      ...(rangeOn ? range : {}),
    })
    if ("error" in plan) {
      failed(plan.error)
      return
    }
    createRequest.mutate(plan.input, {
      onSuccess: () => {
        history.record(withCommand("investigate", trimmed))
        succeeded("Sent to the agent")
        void queryClient.invalidateQueries({
          queryKey: dashboardKeys.revisions(dashboardId),
        })
      },
      onError: (error) => failed(investigateFailureText(error)),
    })
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
    if (event.key === "Backspace" && draft === "" && investigating) {
      event.preventDefault()
      setAgentKind("revise")
    }
  }

  const chips: ContextChip[] = investigating
    ? [
        {
          id: "command",
          label: commandLabels.investigate,
          onRemove: () => {
            setAgentKind("revise")
            focusField()
          },
        },
        ...(rangeOn
          ? [
              {
                id: "range",
                label: describeRange(range.from, range.to),
                onRemove: () => {
                  setRangeOn(false)
                  focusField()
                },
              },
            ]
          : []),
      ]
    : []

  const describedBy =
    [showCounter ? counterId : null, failure ? errorId : null]
      .filter(Boolean)
      .join(" ") || undefined
  const targetItems = panelItems("The whole dashboard", panels)

  return (
    <form
      aria-label="Add to the activity"
      onSubmit={onSubmit}
      className="flex flex-col gap-2 border-t p-3"
    >
      {notice}
      <div className="rounded-card border border-input bg-background focus-within:border-ring focus-within:ring-2 focus-within:ring-ring/50 has-aria-invalid:border-destructive">
        {chips.length > 0 ? (
          <div className="px-3 pt-2">
            <ContextChips chips={chips} />
          </div>
        ) : null}
        <Textarea
          ref={field}
          id={textareaId}
          aria-label="Comment or request"
          aria-invalid={over || undefined}
          aria-describedby={describedBy}
          placeholder={
            !investigating
              ? placeholder
              : panelId === null
                ? "Ask a question about this dashboard"
                : "Ask a question about this panel"
          }
          value={draft}
          readOnly={pending}
          className="field-sizing-content max-h-64 min-h-16 resize-none border-0 px-3 py-2.5 text-[13px] shadow-none focus-visible:ring-0 aria-invalid:ring-0 aria-invalid:focus-visible:ring-0"
          onChange={(event) => {
            onDraftChange(applyText(event.target.value))
            setCursor(freshCursor)
            if (failure) setFailure(null)
          }}
          onKeyDown={onKeyDown}
        />
        <div className="flex items-center gap-2 px-2 pb-2">
          <label htmlFor={targetId} className="text-xs text-muted-foreground">
            On
          </label>
          <Select
            items={targetItems}
            value={panelId ?? noPanelValue}
            onValueChange={(next) => {
              if (next === null) return
              onTargetChange(
                next === noPanelValue
                  ? { kind: "dashboard" }
                  : { kind: "panel", panelId: next }
              )
            }}
          >
            <SelectTrigger
              id={targetId}
              size="sm"
              aria-label="On"
              className="max-w-[240px]"
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent alignItemWithTrigger={false} align="start">
              {targetItems.map((item) => (
                <SelectItem key={item.value} value={item.value}>
                  {item.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <div className="ml-auto flex min-w-0 items-center gap-2">
            <MentionButton
              people={[]}
              field={field}
              value={draft}
              onChange={onDraftChange}
              disabled={pending}
            />
            {model !== null ? (
              <span className="truncate text-xs text-muted-foreground">
                {model}
              </span>
            ) : null}
          </div>
        </div>
      </div>
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
      {gate.kind === "blocked" ? (
        <p className="text-xs text-muted-foreground">{gate.reason}</p>
      ) : null}
      {failure ? (
        <p id={errorId} role="alert" className="text-xs text-status-critical">
          {failure}
        </p>
      ) : null}
      <div className="flex items-center justify-end gap-2">
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
            {investigating ? "Investigate" : "Send to agent"}
          </Button>
        ) : null}
      </div>
      <span role="status" className="sr-only">
        {liveRegionText(note.text, note.count)}
      </span>
    </form>
  )
}
