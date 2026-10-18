import {
  useEffect,
  useId,
  useRef,
  useState,
  type FormEvent,
  type KeyboardEvent,
  type ReactNode,
} from "react"

import { ApiError } from "@/api/errors"
import { isTransient } from "@/api/issues"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { cn } from "@/lib/utils"

import { useCreateComment, type Comment } from "./queries"

export const maxCommentLength = 4000
const counterThreshold = 500

type Props = {
  dashboardId: string
  panelId: string
  panelTitle: string
  version: number
  requestRevision?: boolean
  extraFields?: ReactNode
  onCreated?: (comment: Comment) => void
  onDirtyChange?: (dirty: boolean) => void
}

function failureText(error: unknown, version: number): string {
  if (error instanceof ApiError) {
    if (error.status === 422 && error.code === "unknown_panel") {
      return `This panel is not in version ${version}. Open the latest version to comment.`
    }
    if (
      error.status >= 500 ||
      (error.issues.length > 0 && error.issues.every(isTransient))
    ) {
      return "The comment could not be posted right now. Try again."
    }
    return error.message
  }
  return "The comment could not be posted. Check your connection and try again."
}

function counterText(remaining: number): string {
  if (remaining < 0) {
    const over = -remaining
    return `${over} ${over === 1 ? "character" : "characters"} over the limit`
  }
  return `${remaining} ${remaining === 1 ? "character" : "characters"} left`
}

export function CommentComposer({
  dashboardId,
  panelId,
  panelTitle,
  version,
  requestRevision = false,
  extraFields,
  onCreated,
  onDirtyChange,
}: Props) {
  const textId = useId()
  const counterId = useId()
  const errorId = useId()
  const textarea = useRef<HTMLTextAreaElement>(null)
  const create = useCreateComment(dashboardId)
  const [body, setBody] = useState("")
  const [failure, setFailure] = useState<string | null>(null)

  const trimmed = body.trim()
  const length = [...trimmed].length
  const remaining = maxCommentLength - length
  const over = remaining < 0
  const dirty = trimmed !== ""
  const pending = create.isPending
  const canSubmit = dirty && !over && !pending

  useEffect(() => {
    onDirtyChange?.(dirty)
  }, [dirty, onDirtyChange])
  useEffect(() => () => onDirtyChange?.(false), [onDirtyChange])

  function submit() {
    if (!canSubmit) return
    setFailure(null)
    create.mutate(
      { version, panelId, body: trimmed, requestRevision },
      {
        onSuccess: (comment) => {
          setBody("")
          textarea.current?.focus()
          onCreated?.(comment)
        },
        onError: (error) => setFailure(failureText(error, version)),
      }
    )
  }

  function onSubmit(event: FormEvent) {
    event.preventDefault()
    submit()
  }

  function onKeyDown(event: KeyboardEvent<HTMLTextAreaElement>) {
    if (
      event.key === "Enter" &&
      (event.ctrlKey || event.metaKey) &&
      !event.nativeEvent.isComposing
    ) {
      event.preventDefault()
      submit()
    }
  }

  const showCounter = remaining < counterThreshold
  const describedBy =
    [showCounter ? counterId : null, failure ? errorId : null]
      .filter(Boolean)
      .join(" ") || undefined

  return (
    <form onSubmit={onSubmit} className="flex flex-col gap-2">
      <Label htmlFor={textId} className="wrap-anywhere">
        Comment on {panelTitle}
      </Label>
      <Textarea
        id={textId}
        ref={textarea}
        rows={3}
        value={body}
        readOnly={pending}
        aria-invalid={over || undefined}
        aria-describedby={describedBy}
        className="max-h-[calc(8lh+1rem+2px)] min-h-[calc(3lh+1rem+2px)] overflow-y-auto"
        onChange={(event) => {
          setBody(event.target.value)
          if (failure) setFailure(null)
        }}
        onKeyDown={onKeyDown}
      />
      {failure ? (
        <Alert variant="destructive" id={errorId}>
          <AlertDescription className="text-pretty">{failure}</AlertDescription>
        </Alert>
      ) : null}
      {extraFields}
      <div className="flex items-center justify-between gap-3">
        {showCounter ? (
          <p
            id={counterId}
            className={cn(
              "text-xs text-muted-foreground tabular-nums",
              over && "text-destructive"
            )}
          >
            {counterText(remaining)}
          </p>
        ) : null}
        <Button type="submit" disabled={!canSubmit} className="ml-auto">
          Comment
        </Button>
      </div>
    </form>
  )
}
