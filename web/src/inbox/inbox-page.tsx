import {
  useEffect,
  useRef,
  useSyncExternalStore,
  type KeyboardEvent,
} from "react"
import { useInfiniteQuery } from "@tanstack/react-query"
import { useRouter } from "@tanstack/react-router"
import { toast } from "sonner"

import { ApiError } from "@/api/errors"
import type { InboxFilter, InboxView } from "@/api/keys"
import { EmptyState } from "@/components/empty-state"
import { LoadProblem } from "@/components/load-problem"
import { PageHeader } from "@/components/page-header"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { sentence } from "@/observe/queries"
import { loadProblemText } from "@/service-accounts/queries"

import { InboxDetail } from "./inbox-detail"
import {
  groupByTarget,
  groupItems,
  itemLink,
  type InboxItem,
} from "./inbox-item"
import { InboxRow } from "./inbox-row"
import {
  inboxFilters,
  inboxViews,
  isInboxFilter,
  isInboxView,
} from "./inbox-views"
import {
  inboxInfiniteQuery,
  useInboxLive,
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

const detailQuery = "(min-width: 1100px)"

function subscribeDetail(onChange: () => void) {
  const media = window.matchMedia(detailQuery)
  media.addEventListener("change", onChange)
  return () => media.removeEventListener("change", onChange)
}

function useDetailPane(): boolean {
  return useSyncExternalStore(
    subscribeDetail,
    () => window.matchMedia(detailQuery).matches,
    () => false
  )
}

export type InboxState = {
  view: InboxView
  filter: InboxFilter
  grouped: boolean
  item: string | undefined
}

type Props = Omit<InboxState, "item"> & {
  selected: string | undefined
  onChange: (next: InboxState) => void
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

const filterEmptyText: Record<Exclude<InboxFilter, "open">, string> = {
  unread: "Nothing unread.",
  done: "Nothing marked done.",
}

export function InboxPage({
  view,
  filter,
  grouped,
  selected,
  onChange,
}: Props) {
  useInboxLive()
  const router = useRouter()
  const wide = useDetailPane()
  const list = useRef<HTMLDivElement>(null)
  const query = useInfiniteQuery(inboxInfiniteQuery(view, filter))
  const markRead = useMarkInboxRead()
  const setUnread = useSetInboxItemUnread()
  const setDone = useSetInboxItemDone()
  const opened = useRef<number | null>(null)
  const items = query.data?.pages.flatMap((page) => page.items) ?? []
  const groups = grouped ? groupByTarget(items) : groupItems(items)
  const ordered = groups.flatMap((group) => group.items)
  const unread = query.data?.pages[0]?.unread_count ?? 0
  const newest =
    items.length === 0 ? null : Math.max(...items.map((i) => i.seq))
  const selectedItem =
    items.find((item) => String(item.seq) === selected) ?? null
  const tabStop = selectedItem ?? ordered[0] ?? null

  function select(item: InboxItem) {
    onChange({ view, filter, grouped, item: String(item.seq) })
  }

  function toggleDone(item: InboxItem) {
    const done = filter !== "done"
    setDone.mutate(
      { seq: item.seq, done },
      {
        onSuccess: () => {
          if (done) {
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
          }
        },
        onError: (error) => toast.error(loadProblemText(error)),
      }
    )
  }

  function markOpened(item: InboxItem) {
    if (item.unread) setUnread.mutate({ seq: item.seq, unread: false })
  }

  function toggleUnread(item: InboxItem) {
    opened.current = item.seq
    setUnread.mutate(
      { seq: item.seq, unread: !item.unread },
      { onError: (error) => toast.error(loadProblemText(error)) }
    )
  }

  const selectedSeq = wide ? (selectedItem?.seq ?? null) : null
  const selectedUnread = selectedItem?.unread === true
  useEffect(() => {
    if (selectedSeq === null || opened.current === selectedSeq) return
    opened.current = selectedSeq
    if (selectedUnread) setUnread.mutate({ seq: selectedSeq, unread: false })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedSeq])

  function markAllRead() {
    if (newest === null) return
    markRead.mutate(newest, {
      onSuccess: () => toast.success("Marked as read"),
      onError: (error) => toast.error(loadProblemText(error)),
    })
  }

  function onKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    const key = event.key
    if (key !== "ArrowDown" && key !== "ArrowUp" && key !== "Enter") return
    const rows = Array.from(
      list.current?.querySelectorAll<HTMLElement>("[data-inbox-row]") ?? []
    )
    const target = (event.target as HTMLElement).closest<HTMLElement>(
      "[data-inbox-row]"
    )
    const here = target === null ? -1 : rows.indexOf(target)
    if (here < 0) return
    if (key === "Enter") {
      const link = wide ? itemLink(ordered[here]) : null
      if (link === null) return
      event.preventDefault()
      void router.navigate(link)
      return
    }
    event.preventDefault()
    const next = Math.min(
      rows.length - 1,
      Math.max(0, here + (key === "ArrowDown" ? 1 : -1))
    )
    rows[next].focus()
    if (wide) select(ordered[next])
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
          detail={sentence(query.error.message)}
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
      <div ref={list} onKeyDown={onKeyDown} className="flex flex-col gap-1">
        {groups.map((group) => (
          <section key={group.key} className="flex flex-col gap-1">
            <h2 className="px-3 pt-3 pb-1 text-xs text-muted-foreground">
              {group.label}
            </h2>
            <ul className="flex flex-col gap-0.5">
              {group.items.map((item) => (
                <li key={item.seq}>
                  <InboxRow
                    item={item}
                    selected={wide && item.seq === selectedItem?.seq}
                    tabStop={item.seq === tabStop?.seq}
                    asLink={!wide}
                    onSelect={select}
                    onOpen={markOpened}
                    onToggleUnread={toggleUnread}
                    onToggleDone={toggleDone}
                    done={filter === "done"}
                  />
                </li>
              ))}
            </ul>
          </section>
        ))}
        {query.hasNextPage ? (
          <Button
            variant="outline"
            size="sm"
            className="mt-2 w-fit"
            disabled={query.isFetchingNextPage}
            onClick={() => void query.fetchNextPage()}
          >
            Load more
          </Button>
        ) : null}
      </div>
    )
  }

  return (
    <>
      <PageHeader
        title="Inbox"
        actions={
          unread > 0 && newest !== null ? (
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
      />
      <div className="flex items-start gap-6">
        <div className={wide ? "w-[420px] flex-none" : "w-full min-w-0"}>
          <Tabs
            value={view}
            onValueChange={(next) => {
              if (isInboxView(next)) {
                onChange({ view: next, filter, grouped, item: undefined })
              }
            }}
          >
            <TabsList variant="line" aria-label="Filter the inbox">
              {inboxViews.map((entry) => (
                <TabsTrigger key={entry.value} value={entry.value}>
                  {entry.label}
                </TabsTrigger>
              ))}
            </TabsList>
          </Tabs>
          <div className="mt-3 flex flex-wrap items-center gap-2">
            <div
              role="group"
              aria-label="Show"
              className="flex items-center gap-1"
            >
              {inboxFilters.map((entry) => (
                <Button
                  key={entry.value}
                  type="button"
                  size="sm"
                  variant={filter === entry.value ? "secondary" : "ghost"}
                  aria-pressed={filter === entry.value}
                  onClick={() => {
                    if (isInboxFilter(entry.value)) {
                      onChange({
                        view,
                        filter: entry.value,
                        grouped,
                        item: undefined,
                      })
                    }
                  }}
                >
                  {entry.label}
                </Button>
              ))}
            </div>
            <Button
              type="button"
              size="sm"
              variant={grouped ? "secondary" : "ghost"}
              aria-pressed={grouped}
              onClick={() =>
                onChange({ view, filter, grouped: !grouped, item: selected })
              }
            >
              Group by dashboard or observation
            </Button>
          </div>
          <div className="mt-3">{body()}</div>
        </div>
        {wide ? (
          <section
            aria-label="Selected item"
            className="min-w-0 flex-1 border-l pl-6"
          >
            <InboxDetail
              item={selectedItem}
              done={filter === "done"}
              onToggleUnread={toggleUnread}
              onToggleDone={toggleDone}
            />
          </section>
        ) : null}
      </div>
    </>
  )
}
