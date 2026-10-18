import { useCallback, useMemo, useState, type ReactNode } from "react"
import { useQuery, useQueryClient } from "@tanstack/react-query"

import { Button } from "@/components/ui/button"
import { asDashboardSpec, latestDashboardQuery } from "@/dashboards/queries"

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
import { useComments, type Comment } from "./queries"

type PanelRef = { id: string; title: string }

type Props = {
  dashboardId: string
  version: number
  latestVersion: number
  onOpenPanel: (panelId: string) => void
}

const emptyText: Record<CommentFilter, string> = {
  open: "No open comments on this dashboard.",
  resolved: "No resolved comments.",
  all: "No comments yet.",
}

function useLatestPanels(dashboardId: string): PanelRef[] | undefined {
  const queryClient = useQueryClient()
  const select = useCallback(
    (envelope: { version: { spec: unknown } }): PanelRef[] =>
      asDashboardSpec(envelope.version.spec).panels.map((panel) => ({
        id: panel.id,
        title: panel.title,
      })),
    []
  )
  const query = useQuery({
    ...latestDashboardQuery(queryClient, dashboardId),
    staleTime: Infinity,
    select,
  })
  return query.data
}

function groupByPanel(
  comments: Comment[],
  latest: PanelRef[] | undefined
): { id: string; title: string; present: boolean; comments: Comment[] }[] {
  const byPanel = new Map<string, Comment[]>()
  for (const comment of comments) {
    const list = byPanel.get(comment.panel_id)
    if (list) list.push(comment)
    else byPanel.set(comment.panel_id, [comment])
  }
  const groups = []
  for (const panel of latest ?? []) {
    const list = byPanel.get(panel.id)
    if (!list) continue
    byPanel.delete(panel.id)
    groups.push({
      id: panel.id,
      title: panel.title,
      present: true,
      comments: list,
    })
  }
  for (const [id, list] of byPanel) {
    groups.push({
      id,
      title: latest ? `${id} (removed)` : id,
      present: false,
      comments: list,
    })
  }
  return groups
}

export function DashboardComments({
  dashboardId,
  version,
  latestVersion,
  onOpenPanel,
}: Props) {
  const { comments, isPending, isError, refetch } = useComments(dashboardId)
  const latest = useLatestPanels(dashboardId)
  const [filter, setFilter] = useState<CommentFilter>("open")

  const counts = useMemo(() => countByFilter(comments), [comments])
  const visible = useMemo(
    () => comments.filter((comment) => matchesFilter(comment, filter)),
    [comments, filter]
  )
  const groups = useMemo(() => groupByPanel(visible, latest), [visible, latest])
  const ordered = useMemo(
    () => groups.flatMap((group) => group.comments),
    [groups]
  )
  const { listRef, announcement, onStatusChange } = useCommentFeedback(
    ordered,
    filter
  )

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
                    onStatusChange={onStatusChange}
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
