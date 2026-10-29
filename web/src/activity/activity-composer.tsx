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
import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"
import { useAddComment } from "@/comments/queries"
import { liveRegionText } from "@/lib/live-region"
import { isModEnter } from "@/lib/shortcuts"
import { cn } from "@/lib/utils"
import { sentence } from "@/observe/queries"
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
}

const counterThreshold = 3500
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
}: Props) {
  const counterId = useId()
  const errorId = useId()
  const addComment = useAddComment(dashboardId)
  const requestRevision = useRequestRevision(dashboardId)
  const field = useRef<HTMLTextAreaElement>(null)
  const [failure, setFailure] = useState<string | null>(null)
  const [note, setNote] = useState({ text: "", count: 0 })

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
    announce(text)
    field.current?.focus()
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
        onSuccess: () => succeeded("Sent to the agent"),
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
    }
  }

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
      <Textarea
        ref={field}
        aria-label="Comment or request"
        aria-invalid={over || undefined}
        aria-describedby={describedBy}
        placeholder={placeholder}
        rows={3}
        value={draft}
        readOnly={pending}
        className="field-sizing-fixed max-h-64 min-h-16 resize-y px-3 py-2.5 text-sm"
        onChange={(event) => {
          onDraftChange(event.target.value)
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
        <Button
          type="submit"
          variant="outline"
          disabled={cannotSubmit}
          aria-disabled={pending || undefined}
          aria-busy={pendingComment || undefined}
        >
          Add comment
        </Button>
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
