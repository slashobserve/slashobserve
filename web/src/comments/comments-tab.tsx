import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
  type RefObject,
} from "react"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { liveRegionText } from "@/lib/live-region"
import type { Panel } from "@/spec"

import { CommentComposer } from "./comment-composer"
import { CommentItem } from "./comment-item"
import { useComments, type Comment, type CommentStatus } from "./queries"

export type CommentFilter = CommentStatus | "all"

const filters: { value: CommentFilter; label: string }[] = [
  { value: "open", label: "Open" },
  { value: "resolved", label: "Resolved" },
  { value: "all", label: "All" },
]

const focusWindowMs = 3000

export function matchesFilter(comment: Comment, filter: CommentFilter) {
  return filter === "all" || comment.status === filter
}

export function countByFilter(
  comments: Comment[]
): Record<CommentFilter, number> {
  let open = 0
  for (const comment of comments) if (comment.status === "open") open++
  return { open, resolved: comments.length - open, all: comments.length }
}

export function CommentFilterTabs({
  value,
  onValueChange,
  counts,
  children,
}: {
  value: CommentFilter
  onValueChange: (value: CommentFilter) => void
  counts: Record<CommentFilter, number>
  children: ReactNode
}) {
  return (
    <Tabs
      value={value}
      onValueChange={(next) => onValueChange(next as CommentFilter)}
    >
      <TabsList aria-label="Filter comments">
        {filters.map((filter) => (
          <TabsTrigger key={filter.value} value={filter.value}>
            {filter.label}
            <span className="text-muted-foreground tabular-nums">
              {counts[filter.value]}
            </span>
          </TabsTrigger>
        ))}
      </TabsList>
      {filters.map((filter) => (
        <TabsContent key={filter.value} value={filter.value} className="pt-2">
          {children}
        </TabsContent>
      ))}
    </Tabs>
  )
}

export function CommentsSkeleton() {
  return (
    <div className="flex flex-col gap-3" aria-busy="true">
      <Skeleton className="h-24 w-full animate-none motion-safe:animate-pulse" />
      <Skeleton className="h-24 w-full animate-none motion-safe:animate-pulse" />
    </div>
  )
}

export function CommentsLoadFailed({ onRetry }: { onRetry: () => void }) {
  return (
    <Alert variant="destructive">
      <AlertDescription className="flex flex-wrap items-center justify-between gap-3 text-pretty">
        <span className="min-w-[min(100%,16rem)] flex-1">
          Comments could not be loaded.
        </span>
        <Button
          variant="outline"
          size="sm"
          className="shrink-0 text-foreground"
          onClick={onRetry}
        >
          Try again
        </Button>
      </AlertDescription>
    </Alert>
  )
}

export function useCommentFeedback(
  visible: Comment[],
  filter: CommentFilter
): {
  listRef: RefObject<HTMLDivElement | null>
  announcement: string
  announce: (text: string) => void
  onStatusChange: (comment: Comment) => void
  onStatusSaved: (status: CommentStatus) => void
} {
  const listRef = useRef<HTMLDivElement>(null)
  const [announced, setAnnounced] = useState({ text: "", count: 0 })
  const pending = useRef<{
    leaving: string
    target: string | null
    until: number
  } | null>(null)

  const onStatusChange = useCallback(
    (comment: Comment) => {
      if (filter === "all") return
      const index = visible.findIndex((item) => item.id === comment.id)
      const neighbour = visible[index + 1] ?? visible[index - 1]
      pending.current = {
        leaving: comment.id,
        target: neighbour?.id ?? null,
        until: Date.now() + focusWindowMs,
      }
    },
    [filter, visible]
  )

  useEffect(() => {
    const wait = pending.current
    if (!wait) return
    if (Date.now() > wait.until) {
      pending.current = null
      return
    }
    if (visible.some((item) => item.id === wait.leaving)) return
    pending.current = null
    const list = listRef.current
    if (!list) return
    const target =
      wait.target === null
        ? null
        : list.querySelector<HTMLElement>(
            `[data-comment-id="${CSS.escape(wait.target)}"]`
          )
    ;(target ?? list).focus()
  }, [visible])

  const announce = useCallback((text: string) => {
    setAnnounced((current) => ({ text, count: current.count + 1 }))
  }, [])
  const onStatusSaved = useCallback(
    (status: CommentStatus) =>
      announce(
        status === "resolved" ? "Comment resolved." : "Comment reopened."
      ),
    [announce]
  )

  return {
    listRef,
    announcement: liveRegionText(announced.text, announced.count),
    announce,
    onStatusChange,
    onStatusSaved,
  }
}

const emptyText: Record<CommentFilter, string> = {
  open: "No open comments on this panel.",
  resolved: "No resolved comments.",
  all: "No comments yet.",
}

type Props = {
  dashboardId: string
  panel: Panel
  version: number
  latestVersion: number
  panelRemoved?: boolean
  requestRevision?: boolean
  composerExtraFields?: ReactNode
  renderRevision?: (comment: Comment) => ReactNode
  onDirtyChange: (dirty: boolean) => void
}

export function CommentsTab({
  dashboardId,
  panel,
  version,
  latestVersion,
  panelRemoved,
  requestRevision,
  composerExtraFields,
  renderRevision,
  onDirtyChange,
}: Props) {
  const { comments, isPending, isError, refetch } = useComments(dashboardId)
  const [filter, setFilter] = useState<CommentFilter>("open")
  const [revealed, setRevealed] = useState<string | null>(null)

  const own = useMemo(
    () => comments.filter((comment) => comment.panel_id === panel.id),
    [comments, panel.id]
  )
  const counts = useMemo(() => countByFilter(own), [own])
  const visible = useMemo(
    () => own.filter((comment) => matchesFilter(comment, filter)),
    [own, filter]
  )
  const { listRef, announcement, announce, onStatusChange, onStatusSaved } =
    useCommentFeedback(visible, filter)
  const root = useRef<HTMLDivElement>(null)
  const composerBox = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const container = root.current
    const box = composerBox.current
    if (!container || !box) return
    const observer = new ResizeObserver(([entry]) => {
      container.style.setProperty(
        "--comment-composer-height",
        `${entry.borderBoxSize[0].blockSize}px`
      )
    })
    observer.observe(box, { box: "border-box" })
    return () => observer.disconnect()
  }, [])

  useEffect(() => {
    if (revealed === null) return
    const item = listRef.current?.querySelector<HTMLElement>(
      `[data-comment-id="${CSS.escape(revealed)}"]`
    )
    if (!item) return
    item.scrollIntoView({ block: "nearest" })
    setRevealed(null)
  }, [revealed, visible, listRef])

  let list: ReactNode
  if (isPending) list = <CommentsSkeleton />
  else if (isError && comments.length === 0) {
    list = <CommentsLoadFailed onRetry={() => void refetch()} />
  } else if (visible.length === 0) {
    list = (
      <p className="py-6 text-center text-muted-foreground">
        {emptyText[filter]}
      </p>
    )
  } else {
    list = (
      <ul className="flex flex-col gap-3">
        {visible.map((comment) => (
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
    )
  }

  return (
    <div ref={root} className="flex flex-1 flex-col gap-4">
      <div className="flex flex-1 flex-col">
        <CommentFilterTabs
          value={filter}
          onValueChange={setFilter}
          counts={counts}
        >
          <div ref={listRef} tabIndex={-1} className="outline-none">
            {list}
          </div>
        </CommentFilterTabs>
      </div>
      <p role="status" className="sr-only">
        {announcement}
      </p>
      <div
        ref={composerBox}
        className="sticky -bottom-4 -mx-4 -mb-4 border-t bg-background px-4 py-3"
      >
        <CommentComposer
          dashboardId={dashboardId}
          panelId={panel.id}
          panelTitle={panel.title}
          version={version}
          latestVersion={latestVersion}
          panelRemoved={panelRemoved}
          requestRevision={requestRevision}
          extraFields={composerExtraFields}
          onDirtyChange={onDirtyChange}
          onCreated={(comment) => {
            announce("Comment posted.")
            setFilter((current) => (current === "resolved" ? "open" : current))
            setRevealed(comment.id)
          }}
        />
      </div>
    </div>
  )
}
