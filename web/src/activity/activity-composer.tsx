import {
  useEffect,
  useId,
  useImperativeHandle,
  useRef,
  useState,
  type FormEvent,
  type KeyboardEvent,
  type Ref,
} from "react"
import { useQueryClient } from "@tanstack/react-query"

import { ApiError } from "@/api/errors"
import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"
import { useAddComment } from "@/comments/queries"
import { liveRegionText } from "@/lib/live-region"
import { cn } from "@/lib/utils"
import { sentence } from "@/observe/queries"
import {
  describeRevisionError,
  isAgentUnavailable,
  markAgentUnavailable,
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
}

const maxLength = 4000
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
}: Props) {
  const counterId = useId()
  const errorId = useId()
  const queryClient = useQueryClient()
  const addComment = useAddComment(dashboardId)
  const requestRevision = useRequestRevision(dashboardId)
  const field = useRef<HTMLTextAreaElement>(null)
  const [failure, setFailure] = useState<string | null>(null)
  const [note, setNote] = useState({ text: "", count: 0 })

  useImperativeHandle(textareaRef, () => field.current as HTMLTextAreaElement)

  const targetMissing =
    target.kind === "panel" &&
    !panels.some((panel) => panel.id === target.panelId)
  useEffect(() => {
    if (targetMissing) onTargetChange({ kind: "dashboard" })
  }, [targetMissing, onTargetChange])

  const panelId =
    target.kind === "panel" && !targetMissing ? target.panelId : null
  const trimmed = draft.trim()
  const length = [...draft].length
  const over = length > maxLength
  const showCounter = length >= counterThreshold
  const pendingComment = addComment.isPending
  const pendingRequest = requestRevision.isPending
  const canSubmit =
    trimmed !== "" && !over && !pendingComment && !pendingRequest

  function announce(text: string) {
    setNote((current) => ({ text, count: current.count + 1 }))
  }

  function succeeded(text: string) {
    onDraftChange("")
    announce(text)
    field.current?.focus()
  }

  function addCommentNow() {
    if (!canSubmit) return
    setFailure(null)
    addComment.mutate(
      { body: trimmed, version: latestVersion, panelId },
      {
        onSuccess: () => succeeded("Comment added"),
        onError: (error) => setFailure(commentFailureText(error)),
      }
    )
  }

  function sendToAgent() {
    if (!canSubmit) return
    setFailure(null)
    requestRevision.mutate(
      { prompt: trimmed, baseVersion: latestVersion, panelId },
      {
        onSuccess: () => succeeded("Sent to the agent"),
        onError: (error) => {
          if (isAgentUnavailable(error)) markAgentUnavailable(queryClient)
          setFailure(requestFailureText(error))
        },
      }
    )
  }

  function onSubmit(event: FormEvent) {
    event.preventDefault()
    addCommentNow()
  }

  function onKeyDown(event: KeyboardEvent<HTMLTextAreaElement>) {
    if (
      event.key === "Enter" &&
      (event.ctrlKey || event.metaKey) &&
      !event.nativeEvent.isComposing
    ) {
      event.preventDefault()
      addCommentNow()
    }
  }

  const mac =
    typeof navigator !== "undefined" && navigator.platform.includes("Mac")
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
      <label className="flex items-center gap-2 text-xs text-muted-foreground">
        To
        <select
          value={panelId ?? ""}
          onChange={(event) =>
            onTargetChange(
              event.target.value === ""
                ? { kind: "dashboard" }
                : { kind: "panel", panelId: event.target.value }
            )
          }
          className="h-[26px] max-w-[240px] rounded-control border bg-background px-1.5 text-xs text-foreground"
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
        className="field-sizing-content max-h-40 min-h-16 resize-none text-[13px]"
        onChange={(event) => {
          onDraftChange(event.target.value)
          if (failure) setFailure(null)
        }}
        onKeyDown={onKeyDown}
      />
      {showCounter ? (
        <span
          id={counterId}
          aria-live="polite"
          className={cn(
            "text-xs text-muted-foreground tabular-nums",
            over && "text-status-critical"
          )}
        >
          {over
            ? `${length - maxLength} ${plural(length - maxLength)} over`
            : `${maxLength - length} ${plural(maxLength - length)} left`}
        </span>
      ) : null}
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span className="text-xs text-muted-foreground narrow:hidden">
          {mac ? "⌘ Enter to add" : "Ctrl+Enter to add"}
        </span>
        <div className="ml-auto flex items-center gap-2">
          <Button
            type="submit"
            variant="outline"
            className="h-[30px]"
            disabled={!canSubmit}
            aria-busy={pendingComment || undefined}
          >
            Add comment
          </Button>
          {gate.kind === "allowed" ? (
            <Button
              type="button"
              className="h-[30px]"
              disabled={!canSubmit}
              aria-busy={pendingRequest || undefined}
              onClick={sendToAgent}
            >
              Send to agent
            </Button>
          ) : null}
        </div>
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
