import {
  useEffect,
  useId,
  useRef,
  useState,
  type FormEvent,
  type KeyboardEvent,
} from "react"
import { SendIcon } from "lucide-react"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { useCoarsePointer } from "@/lib/coarse-pointer"
import { cn } from "@/lib/utils"

import { useFollowUpStore } from "./follow-up-store"
import {
  describeRevisionError,
  isActive,
  isAgentUnavailable,
  maxPromptLength,
  useAgentAvailable,
  useRequestRevision,
  useRevisionLookup,
} from "./queries"
import { RevisionList, useHasRevisionList } from "./revision-list"
import { RevisionStatus } from "./revision-status"

const counterThreshold = 500

function counterText(remaining: number): string {
  if (remaining < 0) {
    const over = -remaining
    return `${over} ${over === 1 ? "character" : "characters"} over the limit`
  }
  return `${remaining} ${remaining === 1 ? "character" : "characters"} left`
}

type Props = {
  dashboardId: string
  version: number
  panels: { id: string; title: string }[]
  canAsk: boolean
}

export function FollowUpBar({ dashboardId, version, panels, canAsk }: Props) {
  const available = useAgentAvailable()
  const request = useRequestRevision(dashboardId)
  const coarse = useCoarsePointer()
  const draft = useFollowUpStore((state) => state.drafts[dashboardId] ?? "")
  const trackedId = useFollowUpStore((state) => state.tracked[dashboardId])
  const setDraft = useFollowUpStore((state) => state.setDraft)
  const track = useFollowUpStore((state) => state.track)
  const watch = useFollowUpStore((state) => state.watch)
  const listed = useHasRevisionList(dashboardId)
  const revisions = useRevisionLookup(
    dashboardId,
    trackedId === undefined ? [] : [trackedId]
  )
  const [failure, setFailure] = useState<string | null>(null)
  const [refused, setRefused] = useState(false)
  const field = useRef<HTMLTextAreaElement>(null)
  const notice = useRef<HTMLDivElement>(null)
  const inputId = useId()
  const hintId = useId()
  const counterId = useId()
  const errorId = useId()

  const trimmed = draft.trim()
  const remaining = maxPromptLength - [...trimmed].length
  const over = remaining < 0
  const pending = request.isPending
  const canSubmit = trimmed !== "" && !over && !pending
  const showCounter = remaining < counterThreshold

  useEffect(() => {
    if (!available && refused) notice.current?.focus()
  }, [available, refused])

  const tracked =
    trackedId === undefined
      ? undefined
      : (revisions.get(trackedId) ?? {
          id: trackedId,
          status: "queued" as const,
        })

  function submit() {
    if (!canSubmit) return
    setFailure(null)
    request.mutate(
      { prompt: trimmed, baseVersion: version },
      {
        onSuccess: (revision) => {
          setDraft(dashboardId, "")
          track(dashboardId, revision.id)
          watch(dashboardId, revision.id)
          field.current?.focus()
        },
        onError: (error) => {
          setRefused(isAgentUnavailable(error))
          setFailure(describeRevisionError(error))
        },
      }
    )
  }

  function onSubmit(event: FormEvent) {
    event.preventDefault()
    submit()
  }

  function onKeyDown(event: KeyboardEvent<HTMLTextAreaElement>) {
    if (
      !coarse &&
      event.key === "Enter" &&
      !event.shiftKey &&
      !event.nativeEvent.isComposing
    ) {
      event.preventDefault()
      submit()
    }
  }

  const describedBy =
    [
      failure ? errorId : null,
      showCounter ? counterId : null,
      coarse ? null : hintId,
    ]
      .filter(Boolean)
      .join(" ") || undefined

  const showForm = canAsk && available
  const showNotice = canAsk && !available && refused
  if (!showForm && !showNotice && !listed) return null

  return (
    <div
      className={cn(
        "flex flex-col gap-2 border-b px-4",
        showForm ? "py-3" : "py-2"
      )}
    >
      {showForm || listed ? (
        <div className="flex items-center justify-end gap-2">
          {showForm ? (
            <Label htmlFor={inputId} className="mr-auto">
              Ask for a change
            </Label>
          ) : null}
          {listed ? (
            <RevisionList dashboardId={dashboardId} panels={panels} />
          ) : null}
        </div>
      ) : null}
      {showNotice ? (
        <Alert ref={notice} tabIndex={-1} className="outline-none">
          <AlertDescription>
            {describeRevisionError(request.error)}
          </AlertDescription>
        </Alert>
      ) : null}
      {showForm ? (
        <form onSubmit={onSubmit} className="flex flex-col gap-2">
          <div className="flex flex-col gap-2 sm:flex-row sm:items-start">
            <Textarea
              ref={field}
              id={inputId}
              rows={1}
              value={draft}
              readOnly={pending}
              aria-invalid={over || undefined}
              aria-describedby={describedBy}
              enterKeyHint={coarse ? "enter" : "send"}
              placeholder="Describe a change, for example add a p99 latency panel"
              className="max-h-[calc(4lh+12px)] min-h-8 resize-none overflow-y-auto py-[5px] leading-5 sm:flex-1"
              onChange={(event) => {
                setDraft(dashboardId, event.target.value)
                if (failure) setFailure(null)
              }}
              onKeyDown={onKeyDown}
            />
            <Button
              type="submit"
              disabled={!canSubmit}
              aria-busy={pending || undefined}
              className="max-sm:w-full"
            >
              <SendIcon data-icon="inline-start" />
              Ask agent
            </Button>
          </div>
          {failure ? (
            <Alert variant="destructive" id={errorId}>
              <AlertDescription className="text-pretty">
                {failure}
              </AlertDescription>
            </Alert>
          ) : null}
          <div className="flex min-h-5 flex-wrap items-center justify-between gap-x-4 gap-y-1 text-xs text-muted-foreground">
            {coarse ? null : (
              <p id={hintId}>Enter sends. Shift+Enter adds a new line.</p>
            )}
            {showCounter ? (
              <p
                id={counterId}
                className={cn(
                  "ml-auto tabular-nums",
                  over && "text-destructive"
                )}
              >
                {counterText(remaining)}
              </p>
            ) : null}
          </div>
          {tracked ? (
            <div className="flex flex-wrap items-start justify-between gap-x-4 gap-y-1">
              <RevisionStatus
                live
                revision={tracked}
                dashboardId={dashboardId}
              />
              {isActive(tracked.status) ? null : (
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  onClick={() => {
                    track(dashboardId, null)
                    field.current?.focus()
                  }}
                >
                  Dismiss
                </Button>
              )}
            </div>
          ) : null}
        </form>
      ) : null}
    </div>
  )
}
