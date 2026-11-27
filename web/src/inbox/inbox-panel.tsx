import {
  useCallback,
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type KeyboardEvent,
  type MouseEvent,
  type RefObject,
} from "react"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"

import { useActivityStore, usePanelLayout } from "@/activity/activity-store"
import { ApiError, sentence, loadProblemText } from "@/api/errors"
import type { InboxFilter, InboxView } from "@/api/keys"
import { DockHeader } from "@/components/dock/dock-header"
import { DockPanel } from "@/components/dock/dock-panel"
import { EmptyState } from "@/components/empty-state"
import { LoadProblem } from "@/components/load-problem"
import { Button } from "@/components/ui/button"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { useDashboardShortcuts } from "@/shortcuts/use-dashboard-shortcuts"

import { groupItems, itemLink, type InboxItem } from "./inbox-item"
import {
  returnFocusFromInbox,
  toggleInboxFromKeys,
  useInboxPanelStore,
} from "./inbox-panel-store"
import { InboxRow } from "./inbox-row"
import { inboxFilters, inboxViews } from "./inbox-views"
import { useLiveInboxToasts } from "./use-live-toasts"
import {
  keepInboxHead,
  usePanelInboxList,
  useMarkInboxRead,
  useSetInboxItemDone,
  useSetInboxItemUnread,
} from "./queries"

const emptyText: Record<InboxView, string> = {
  for_you: "Nothing needs you right now.",
  everything: "No activity in the last 30 days.",
  proposals: "No proposals waiting for you.",
  mentions: "No mentions or replies.",
}

const viewsWithMarkAllRead: readonly InboxView[] = ["for_you", "everything"]

const filterEmptyText: Record<Exclude<InboxFilter, "open">, string> = {
  unread: "Nothing unread.",
  done: "Nothing marked done.",
}

function opensInThisTab(event: MouseEvent): boolean {
  return (
    event.button === 0 &&
    !event.metaKey &&
    !event.ctrlKey &&
    !event.shiftKey &&
    !event.altKey
  )
}

function isSignedInOnly(error: unknown): boolean {
  return (
    error instanceof ApiError &&
    error.status === 403 &&
    error.code === "user_session_required"
  )
}

function ListSkeleton() {
  return (
    <div aria-busy="true" className="flex flex-col gap-2">
      {[0, 1, 2, 3, 4].map((row) => (
        <Skeleton key={row} className="h-14 rounded-control" />
      ))}
    </div>
  )
}

export function InboxPanel() {
  const open = useInboxPanelStore((state) => state.open)
  const openings = useInboxPanelStore((state) => state.openings)
  useLiveInboxToasts()
  useDashboardShortcuts({ inbox: toggleInboxFromKeys })
  return open ? <InboxCard key={openings} /> : null
}

function useLoadMoreOnScroll(
  sentinel: RefObject<HTMLElement | null>,
  more: boolean,
  loadMore: () => void
) {
  useEffect(() => {
    const element = sentinel.current
    if (!element || !more) return
    const observer = new IntersectionObserver((entries) => {
      if (entries.some((entry) => entry.isIntersecting)) loadMore()
    })
    observer.observe(element)
    return () => observer.disconnect()
  }, [sentinel, more, loadMore])
}

function InboxCard() {
  const titleId = useId()
  const asideRef = useRef<HTMLElement>(null)
  const titleRef = useRef<HTMLHeadingElement>(null)
  const listRef = useRef<HTMLDivElement>(null)
  const sentinel = useRef<HTMLDivElement>(null)
  const layout = usePanelLayout()
  const setPanelWidth = useActivityStore((state) => state.setPanelWidth)
  const { view, filter, setView, setFilter, close } = useInboxPanelStore()
  const [target] = useState(() => useInboxPanelStore.getState().item)
  const queryClient = useQueryClient()
  const [opened] = useState({ view, filter })
  useLayoutEffect(() => {
    keepInboxHead(queryClient, opened.view, opened.filter)
    return () => keepInboxHead(queryClient, "for_you", "open")
  }, [queryClient, opened])
  const query = usePanelInboxList(view, filter)
  const markRead = useMarkInboxRead()
  const setUnread = useSetInboxItemUnread()
  const setDone = useSetInboxItemDone()
  const [focused, setFocused] = useState<number | null>(target)
  const [expanded, setExpanded] = useState<ReadonlySet<number>>(
    () => new Set(target === null ? [] : [target])
  )
  const revealed = useRef(false)
  const items = query.data?.pages.flatMap((page) => page.items) ?? []
  const groups = groupItems(items)
  const ordered = groups.flatMap((group) => group.items)
  const unread = query.data?.pages[0]?.unread_count ?? 0
  const newest =
    items.length === 0 ? null : Math.max(...items.map((i) => i.seq))
  const tabStop =
    ordered.find((item) => item.seq === focused) ?? ordered[0] ?? null
  const {
    hasNextPage,
    isFetchingNextPage,
    isFetchNextPageError,
    fetchNextPage,
  } = query

  const loadMore = useCallback(() => void fetchNextPage(), [fetchNextPage])
  useLoadMoreOnScroll(
    sentinel,
    hasNextPage && !isFetchingNextPage && !isFetchNextPageError,
    loadMore
  )

  useEffect(() => {
    titleRef.current?.focus()
  }, [])

  const seekingTarget =
    target !== null &&
    query.data !== undefined &&
    !items.some((item) => item.seq === target)
  useEffect(() => {
    if (!seekingTarget || revealed.current) return
    if (hasNextPage && !isFetchingNextPage && !isFetchNextPageError) loadMore()
  }, [
    seekingTarget,
    hasNextPage,
    isFetchingNextPage,
    isFetchNextPageError,
    loadMore,
  ])
  const targetMissing = seekingTarget && !hasNextPage && !revealed.current

  useEffect(() => {
    if (revealed.current || target === null) return
    const row = listRef.current?.querySelector<HTMLElement>(
      `[data-inbox-row="${target}"]`
    )
    if (!row) return
    revealed.current = true
    row.scrollIntoView({ block: "nearest" })
    row.focus()
  })

  function closeToOpener() {
    close()
    returnFocusFromInbox()
  }

  function toggleExpanded(item: InboxItem) {
    markOpened(item)
    setFocused(item.seq)
    setExpanded((current) => {
      const next = new Set(current)
      if (!next.delete(item.seq)) next.add(item.seq)
      return next
    })
  }

  function markOpened(item: InboxItem) {
    if (item.unread) setUnread.mutate({ seq: item.seq, unread: false })
  }

  function openItem(item: InboxItem, event: MouseEvent) {
    markOpened(item)
    if (opensInThisTab(event)) close()
  }

  function toggleUnread(item: InboxItem) {
    setUnread.mutate(
      { seq: item.seq, unread: !item.unread },
      { onError: (error) => toast.error(loadProblemText(error)) }
    )
  }

  function toggleDone(item: InboxItem) {
    const done = item.state !== "done"
    setDone.mutate(
      { seq: item.seq, done },
      {
        onSuccess: () => {
          if (!done) return
          toast.success("Marked done", {
            action: {
              label: "Undo",
              onClick: () =>
                setDone.mutate(
                  { seq: item.seq, done: false },
                  { onError: (error) => toast.error(loadProblemText(error)) }
                ),
            },
          })
        },
        onError: (error) => toast.error(loadProblemText(error)),
      }
    )
  }

  function markAllRead() {
    if (newest === null) return
    markRead.mutate(newest, {
      onSuccess: () => toast.success("Marked as read"),
      onError: (error) => toast.error(loadProblemText(error)),
    })
  }

  function onKeyDown(event: KeyboardEvent<HTMLElement>) {
    if (event.key !== "Escape" || event.defaultPrevented) return
    if (!asideRef.current?.contains(event.target as Node)) return
    event.preventDefault()
    closeToOpener()
  }

  function onListKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (event.key !== "ArrowDown" && event.key !== "ArrowUp") return
    const rows = Array.from(
      listRef.current?.querySelectorAll<HTMLElement>("[data-inbox-row]") ?? []
    )
    const row = (event.target as HTMLElement).closest<HTMLElement>(
      "[data-inbox-row]"
    )
    const here = row === null ? -1 : rows.indexOf(row)
    if (here < 0) return
    event.preventDefault()
    const next = Math.min(
      rows.length - 1,
      Math.max(0, here + (event.key === "ArrowDown" ? 1 : -1))
    )
    rows[next].focus()
    setFocused(ordered[next].seq)
  }

  function body() {
    if (query.isError && query.data === undefined) {
      if (isSignedInOnly(query.error)) {
        return (
          <p className="text-[13px] text-muted-foreground">
            The inbox is for signed-in people.
          </p>
        )
      }
      return (
        <LoadProblem
          title="Could not load the inbox"
          detail={loadProblemText(query.error)}
          retrying={query.isFetching}
          onRetry={() => void query.refetch()}
        />
      )
    }
    if (query.data === undefined) return <ListSkeleton />
    if (groups.length === 0) {
      return (
        <EmptyState
          title={filter === "open" ? emptyText[view] : filterEmptyText[filter]}
        />
      )
    }
    return (
      <div ref={listRef} onKeyDown={onListKeyDown} className="flex flex-col">
        {groups.map((group) => (
          <section key={group.key} className="flex flex-col gap-1">
            <h3 className="px-3 pt-3 pb-1 text-xs text-muted-foreground">
              {group.label}
            </h3>
            <ul className="flex flex-col gap-0.5">
              {group.items.map((item) => (
                <li key={item.seq}>
                  <InboxRow
                    item={item}
                    selected={item.seq === target}
                    tabStop={item.seq === tabStop?.seq}
                    asLink
                    expanded={
                      itemLink(item) === null
                        ? expanded.has(item.seq)
                        : undefined
                    }
                    onSelect={toggleExpanded}
                    onOpen={openItem}
                    onToggleUnread={toggleUnread}
                    onToggleDone={toggleDone}
                    done={item.state === "done"}
                  />
                </li>
              ))}
            </ul>
          </section>
        ))}
        <div ref={sentinel} aria-hidden="true" className="h-px" />
        {isFetchingNextPage ? <ListSkeleton /> : null}
        {isFetchNextPageError && !isFetchingNextPage ? (
          <div className="mt-2 px-1">
            <LoadProblem
              title="Could not load more of the inbox"
              detail={sentence(query.error?.message ?? "")}
              retrying={false}
              onRetry={loadMore}
            />
          </div>
        ) : null}
      </div>
    )
  }

  return (
    <DockPanel
      panelKey="inbox"
      ref={asideRef}
      isInbox
      labelledBy={titleId}
      onKeyDown={onKeyDown}
      width={layout.width}
      bounds={layout.bounds}
      onWidthChange={setPanelWidth}
      stacked={layout.stacked}
    >
      <DockHeader
        headingId={titleId}
        headingRef={titleRef}
        title="Inbox"
        controls={
          viewsWithMarkAllRead.includes(view) &&
          unread > 0 &&
          newest !== null ? (
            <Button
              variant="outline"
              size="sm"
              disabled={markRead.isPending}
              onClick={markAllRead}
            >
              Mark all read
            </Button>
          ) : null
        }
        onClose={closeToOpener}
        closeLabel="Close Inbox"
      />
      <div
        data-slot="dock-toolbar"
        className="flex flex-none flex-wrap items-center gap-2 border-b px-4 py-2.5 group-data-[peek]/dock:hidden"
      >
        <Select
          items={inboxViews}
          value={view}
          onValueChange={(next: InboxView | null) => {
            if (next !== null) setView(next)
          }}
        >
          <SelectTrigger aria-label="Inbox view" size="sm">
            <SelectValue />
          </SelectTrigger>
          <SelectContent alignItemWithTrigger={false} align="start">
            {inboxViews.map((entry) => (
              <SelectItem key={entry.value} value={entry.value}>
                {entry.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select
          items={inboxFilters}
          value={filter}
          onValueChange={(next: InboxFilter | null) => {
            if (next !== null) setFilter(next)
          }}
        >
          <SelectTrigger aria-label="Show" size="sm">
            <SelectValue />
          </SelectTrigger>
          <SelectContent alignItemWithTrigger={false} align="start">
            {inboxFilters.map((entry) => (
              <SelectItem key={entry.value} value={entry.value}>
                {entry.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <div className="relative flex min-h-0 flex-1 flex-col overflow-auto px-2 pb-4">
        {targetMissing ? (
          <p
            role="status"
            className="px-3 pt-3 text-[13px] text-muted-foreground"
          >
            The item this link points to is not in this list any more.
          </p>
        ) : null}
        {body()}
      </div>
    </DockPanel>
  )
}
