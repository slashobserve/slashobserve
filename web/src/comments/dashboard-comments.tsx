import { useMemo, useState, type ReactNode } from "react"

import { Button } from "@/components/ui/button"

import {
  CommentFilterTabs,
  CommentsLoadFailed,
  CommentsSkeleton,
  countByFilter,
  matchesFilter,
  useCommentFeedback,
  type CommentFilter,
} from "./comments-tab"
import { CommentItem } from "./comment-item"
import { useLatestPanels, type PanelRef } from "./latest-panels"
import { useComments, type Comment } from "./queries"

type Props = {
  dashboardId: string
  panels: PanelRef[]
  version: number
  latestVersion: number
  onOpenPanel: (panelId: string) => void
  renderRevision?: (comment: Comment) => ReactNode
}

type Group = {
  id: string
  title: string
  present: boolean
  inLatest: boolean
  comments: Comment[]
}

const emptyText: Record<CommentFilter, string> = {
  open: "No open comments on this dashboard.",
  resolved: "No resolved comments.",
  all: "No comments yet.",
}

function groupByPanel(
  comments: Comment[],
  shown: PanelRef[],
  latest: PanelRef[] | undefined
): Group[] {
  const byPanel = new Map<string, Comment[]>()
  for (const comment of comments) {
    if (comment.panel_id === null) continue
    const list = byPanel.get(comment.panel_id)
    if (list) list.push(comment)
    else byPanel.set(comment.panel_id, [comment])
  }
  const groups: Group[] = []
  for (const panel of shown) {
    const list = byPanel.get(panel.id)
    if (!list) continue
    byPanel.delete(panel.id)
    groups.push({
      id: panel.id,
      title: panel.title,
      present: true,
      inLatest: true,
      comments: list,
    })
  }
  for (const [id, list] of byPanel) {
    const current = latest?.find((panel) => panel.id === id)
    groups.push({
      id,
      title: current ? current.title : latest ? `${id} (removed)` : id,
      present: false,
      inLatest: current !== undefined,
      comments: list,
    })
  }
  return groups
}

export function DashboardComments({
  dashboardId,
  panels,
  version,
  latestVersion,
  onOpenPanel,
  renderRevision,
}: Props) {
  const { comments, isPending, isError, refetch } = useComments(dashboardId)
  const latest = useLatestPanels(dashboardId)
  const [filter, setFilter] = useState<CommentFilter>("open")

  const counts = useMemo(() => countByFilter(comments), [comments])
  const visible = useMemo(
    () => comments.filter((comment) => matchesFilter(comment, filter)),
    [comments, filter]
  )
  const groups = useMemo(
    () => groupByPanel(visible, panels, latest),
    [visible, panels, latest]
  )
  const ordered = useMemo(
    () => groups.flatMap((group) => group.comments),
    [groups]
  )
  const { listRef, announcement, onStatusChange, onStatusSaved } =
    useCommentFeedback(ordered, filter)

  let list: ReactNode
  if (isPending) list = <CommentsSkeleton />
  else if (isError && comments.length === 0) {
    list = <CommentsLoadFailed onRetry={() => void refetch()} />
  } else if (groups.length === 0) {
    list = (
      <p className="py-6 text-center text-muted-foreground">
        {emptyText[filter]}
      </p>
    )
  } else {
    list = (
      <div className="flex flex-col gap-6">
        {groups.map((group) => (
          <section
            key={group.id}
            aria-labelledby={`comment-group-${group.id}`}
            className="flex flex-col gap-3"
          >
            <div className="flex flex-wrap items-center justify-between gap-2">
              <h3
                id={`comment-group-${group.id}`}
                className="min-w-0 font-medium wrap-anywhere"
              >
                {group.title}
              </h3>
              {group.present ? (
                <Button
                  variant="outline"
                  size="sm"
                  aria-label={`Open panel ${group.title}`}
                  onClick={() => onOpenPanel(group.id)}
                >
                  Open panel
                </Button>
              ) : group.inLatest ? (
                <span className="text-xs text-muted-foreground">
                  Not in version {version}
                </span>
              ) : null}
            </div>
            <ul className="flex flex-col gap-3">
              {group.comments.map((comment) => (
                <li key={comment.id}>
                  <CommentItem
                    dashboardId={dashboardId}
                    comment={comment}
                    shownVersion={version}
                    latestVersion={latestVersion}
                    revisionSlot={renderRevision?.(comment)}
                    onStatusChange={onStatusChange}
                    onStatusSaved={onStatusSaved}
                  />
                </li>
              ))}
            </ul>
          </section>
        ))}
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-4">
      <CommentFilterTabs
        value={filter}
        onValueChange={setFilter}
        counts={counts}
      >
        <div ref={listRef} tabIndex={-1} className="outline-none">
          {list}
        </div>
      </CommentFilterTabs>
      <p role="status" className="sr-only">
        {announcement}
      </p>
    </div>
  )
}
