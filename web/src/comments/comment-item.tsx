import { useId, type ReactNode } from "react"
import { Link } from "@tanstack/react-router"
import { CircleSlashIcon } from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { searchFromState, useDashboardSearch } from "@/dashboards/search"
import { formatDateTime, formatRelative } from "@/lib/time"

import {
  useSetCommentStatus,
  type Comment,
  type CommentStatus,
} from "./queries"

type Props = {
  dashboardId: string
  comment: Comment
  shownVersion: number
  latestVersion: number
  revisionSlot?: ReactNode
  onStatusChange?: (comment: Comment) => void
  onStatusSaved?: (status: CommentStatus) => void
}

export function CommentItem({
  dashboardId,
  comment,
  shownVersion,
  latestVersion,
  revisionSlot,
  onStatusChange,
  onStatusSaved,
}: Props) {
  const headingId = useId()
  const search = useDashboardSearch()
  const setStatus = useSetCommentStatus(dashboardId, onStatusSaved)
  const resolved = comment.status === "resolved"
  const next: CommentStatus = resolved ? "open" : "resolved"
  const action = resolved ? "Reopen" : "Resolve"
  const when = formatRelative(comment.created_at)

  return (
    <article
      aria-labelledby={headingId}
      data-comment-id={comment.id}
      tabIndex={-1}
      className="flex scroll-mb-[calc(var(--comment-composer-height,0px)+0.5rem)] flex-col gap-2 rounded-lg border bg-card p-3 text-card-foreground outline-none focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring"
    >
      <div
        id={headingId}
        className="flex flex-wrap items-center gap-x-2 gap-y-1 text-sm"
      >
        <span className="font-medium wrap-anywhere">{comment.author.name}</span>
        <time
          dateTime={comment.created_at}
          title={formatDateTime(comment.created_at)}
          className="text-muted-foreground"
        >
          {when}
        </time>
        {resolved ? <Badge variant="secondary">Resolved</Badge> : null}
      </div>
      <p className="text-xs text-muted-foreground">
        {comment.version === shownVersion ? (
          <>on version {comment.version}</>
        ) : (
          <Link
            to="/dashboards/$dashboardId"
            params={{ dashboardId }}
            search={searchFromState({
              ...search,
              version:
                comment.version === latestVersion ? undefined : comment.version,
              panel: undefined,
              tab: undefined,
            })}
            className="underline underline-offset-3 hover:text-foreground"
          >
            on version {comment.version}
          </Link>
        )}
      </p>
      {comment.panel_present ? null : (
        <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
          <CircleSlashIcon aria-hidden="true" className="size-3.5 shrink-0" />
          This panel is not in the latest version.
        </p>
      )}
      <p className="text-sm break-words whitespace-pre-wrap">{comment.body}</p>
      {revisionSlot}
      <div className="flex justify-end">
        <Button
          variant="ghost"
          size="sm"
          aria-label={`${action} comment by ${comment.author.name}, ${when}`}
          aria-disabled={setStatus.isPending || undefined}
          onClick={() => {
            if (setStatus.isPending) return
            onStatusChange?.(comment)
            setStatus.mutate({ commentId: comment.id, status: next })
          }}
        >
          {action}
        </Button>
      </div>
    </article>
  )
}
