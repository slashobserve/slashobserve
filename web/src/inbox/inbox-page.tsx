import { useRef, useSyncExternalStore, type KeyboardEvent } from "react"
import { useInfiniteQuery } from "@tanstack/react-query"
import { useRouter } from "@tanstack/react-router"
import { toast } from "sonner"

import { ApiError } from "@/api/errors"
import type { InboxView } from "@/api/keys"
import { EmptyState } from "@/components/empty-state"
import { LoadProblem } from "@/components/load-problem"
import { PageHeader } from "@/components/page-header"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { sentence } from "@/observe/queries"
import { loadProblemText } from "@/service-accounts/queries"

import { InboxDetail } from "./inbox-detail"
import { groupItems, itemLink, type InboxItem } from "./inbox-item"
import { InboxRow } from "./inbox-row"
import { inboxInfiniteQuery, useInboxLive, useMarkInboxRead } from "./queries"

const inboxViews: readonly { value: InboxView; label: string }[] = [
  { value: "for_you", label: "For you" },
  { value: "everything", label: "Everything" },
  { value: "proposals", label: "Proposals" },
  { value: "mentions", label: "Mentions" },
]

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

export function isInboxView(value: unknown): value is InboxView {
  return inboxViews.some((view) => view.value === value)
}

type Props = {
  view: InboxView
  selected: string | undefined
  onChange: (next: { view: InboxView; item: string | undefined }) => void
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

export function InboxPage({ view, selected, onChange }: Props) {
  useInboxLive()
  const router = useRouter()
  const wide = useDetailPane()
  const list = useRef<HTMLDivElement>(null)
  const query = useInfiniteQuery(inboxInfiniteQuery(view))
  const markRead = useMarkInboxRead()
  const items = query.data?.pages.flatMap((page) => page.items) ?? []
  const groups = groupItems(items)
  const ordered = groups.flatMap((group) => group.items)
  const unread = query.data?.pages[0]?.unread_count ?? 0
  const newest =
    items.length === 0 ? null : Math.max(...items.map((i) => i.seq))
  const selectedItem =
    items.find((item) => String(item.seq) === selected) ?? null
  const tabStop = selectedItem ?? ordered[0] ?? null

  function select(item: InboxItem) {
    onChange({ view, item: String(item.seq) })
  }

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
    if (groups.length === 0) return <EmptyState title={emptyText[view]} />
    return (
      <div ref={list} onKeyDown={onKeyDown} className="flex flex-col gap-1">
        {groups.map((group) => (
          <section key={group.label} className="flex flex-col gap-1">
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
    <div className="mx-auto w-full max-w-6xl">
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
              if (isInboxView(next)) onChange({ view: next, item: undefined })
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
          <div className="mt-3">{body()}</div>
        </div>
        {wide ? (
          <section
            aria-label="Selected item"
            className="min-w-0 flex-1 border-l pl-6"
          >
            <InboxDetail item={selectedItem} />
          </section>
        ) : null}
      </div>
    </div>
  )
}
