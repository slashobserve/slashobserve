import { useEffect, useId, useMemo, useRef, useState } from "react"
import { Link, useRouteContext } from "@tanstack/react-router"
import { MessageSquareIcon, SearchIcon } from "lucide-react"

import { AuthorMark } from "@/components/author-mark"
import { EmptyState } from "@/components/empty-state"
import { FilterToolbar } from "@/components/filter-toolbar"
import { LoadProblem } from "@/components/load-problem"
import { PageHeader } from "@/components/page-header"
import { RelativeTime, useNow } from "@/components/relative-time"
import { StatusLabel } from "@/components/status-label"
import { Button, buttonVariants } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { useInView } from "@/dashboards/use-in-view"
import { rowLinkClass, RowText } from "@/home/home-section"
import { markOf } from "@/lib/author"
import { roleAtLeast } from "@/lib/roles"
import { cn } from "@/lib/utils"
import { useMyRequests, type RequestSummary } from "@/observe/queries"
import { loadProblemText } from "@/service-accounts/queries"

import { groupChatsByDay } from "./chat-days"
import {
  chatOutcome,
  chatState,
  statesFor,
  type ChatStateFilter,
} from "./chat-state"
import type { ChatsSearch } from "./chats-search"

type Props = {
  search: ChatsSearch
  onSearch: (next: ChatsSearch) => void
}

const pageSize = 30
const searchDelayMs = 300
const skeletonRows = 3
const mineSubtitle =
  "Your chats with Observe Agent, newest first. Only you and workspace admins can see them."
const everyoneSubtitle =
  "Everyone's chats with Observe Agent. Only admins see this view."

type StateItem = ChatStateFilter | "any"

const stateItems: { value: StateItem; label: string }[] = [
  { value: "any", label: "State: any" },
  { value: "running", label: "State: running" },
  { value: "stopped", label: "State: stopped" },
  { value: "done", label: "State: done" },
]

const scopeButtonClass =
  "h-6 rounded-[4px] border border-transparent px-2.5 text-[13px] text-muted-foreground outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring aria-pressed:border-border aria-pressed:bg-card aria-pressed:font-medium aria-pressed:text-foreground narrow:h-11"

export function ChatsPage({ search, onSearch }: Props) {
  const { principal } = useRouteContext({ from: "/_authed" })
  const admin = roleAtLeast(principal.role, "admin")
  const everyone = admin && search.scope === "all"
  const filtered = search.q !== undefined || search.state !== undefined
  const { requests, query } = useMyRequests(
    {
      q: search.q,
      states: search.state ? statesFor(search.state) : undefined,
      scope: everyone ? "all" : undefined,
    },
    pageSize
  )

  const newChat = (
    <Link to="/" className={cn(buttonVariants())}>
      <MessageSquareIcon aria-hidden="true" />
      New chat
    </Link>
  )

  function clearFilters() {
    onSearch({ scope: search.scope })
  }

  return (
    <>
      <PageHeader
        title="Chats"
        description={everyone ? everyoneSubtitle : mineSubtitle}
        actions={newChat}
      />
      <FilterToolbar
        label="Chat filters"
        onClear={filtered ? clearFilters : undefined}
      >
        <SearchField
          value={search.q}
          onCommit={(q) => onSearch({ ...search, q })}
        />
        {admin ? (
          <div
            role="group"
            aria-label="Whose chats"
            className="inline-flex gap-0.5 rounded-control bg-muted p-0.5"
          >
            <button
              type="button"
              aria-pressed={!everyone}
              className={scopeButtonClass}
              onClick={() => onSearch({ ...search, scope: undefined })}
            >
              Mine
            </button>
            <button
              type="button"
              aria-pressed={everyone}
              className={scopeButtonClass}
              onClick={() => onSearch({ ...search, scope: "all" })}
            >
              Everyone
            </button>
          </div>
        ) : null}
        <Select<StateItem>
          items={stateItems}
          value={search.state ?? "any"}
          onValueChange={(next) => {
            if (next === null) return
            onSearch({ ...search, state: next === "any" ? undefined : next })
          }}
        >
          <SelectTrigger aria-label="State" className="w-40">
            <SelectValue />
          </SelectTrigger>
          <SelectContent alignItemWithTrigger={false} align="start">
            {stateItems.map((item) => (
              <SelectItem key={item.value} value={item.value}>
                {item.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </FilterToolbar>
      {query.isError && query.data === undefined ? (
        <LoadProblem
          title="Could not load chats"
          detail={loadProblemText(query.error)}
          retrying={query.isFetching}
          onRetry={() => void query.refetch()}
        />
      ) : query.data === undefined ? (
        <ChatsSkeleton />
      ) : requests.length === 0 ? (
        <div className="rounded-card border bg-background">
          {filtered ? (
            <EmptyState
              title="No chats match these filters."
              action={
                <Button variant="ghost" size="sm" onClick={clearFilters}>
                  Clear filters
                </Button>
              }
            />
          ) : (
            <EmptyState title="No requests yet" action={newChat}>
              Ask for a dashboard to start one.
            </EmptyState>
          )}
        </div>
      ) : (
        <>
          <ChatGroups requests={requests} everyone={everyone} />
          {query.hasNextPage ? (
            <NextPage
              failed={query.isFetchNextPageError}
              fetching={query.isFetchingNextPage}
              onLoad={query.fetchNextPage}
            />
          ) : null}
        </>
      )}
    </>
  )
}

function SearchField({
  value,
  onCommit,
}: {
  value: string | undefined
  onCommit: (q: string | undefined) => void
}) {
  const [text, setText] = useState(value ?? "")
  const [shown, setShown] = useState(value)
  if (value !== shown) {
    setShown(value)
    if ((value ?? "") !== text.trim()) setText(value ?? "")
  }

  useEffect(() => {
    const typed = text.trim()
    if (typed === (value ?? "")) return
    const timer = setTimeout(
      () => onCommit(typed === "" ? undefined : typed),
      searchDelayMs
    )
    return () => clearTimeout(timer)
  }, [text, value, onCommit])

  return (
    <div className="relative min-w-[200px] flex-1">
      <SearchIcon
        aria-hidden="true"
        className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-muted-foreground"
      />
      <Input
        type="search"
        aria-label="Search chats"
        placeholder="Search chats"
        autoComplete="off"
        maxLength={200}
        value={text}
        onChange={(event) => setText(event.target.value)}
        className="pl-8"
      />
    </div>
  )
}

function ChatGroups({
  requests,
  everyone,
}: {
  requests: readonly RequestSummary[]
  everyone: boolean
}) {
  const id = useId()
  const current = useNow()
  const groups = useMemo(
    () => groupChatsByDay(requests, new Date(current), !everyone),
    [requests, current, everyone]
  )
  return (
    <div className="overflow-hidden rounded-card border bg-card">
      {groups.map((group, index) => (
        <section
          key={group.label}
          aria-labelledby={`${id}-${index}`}
          className="border-t first:border-t-0"
        >
          <h2
            id={`${id}-${index}`}
            className="bg-canvas px-4 pt-2.5 pb-1.5 text-xs font-normal text-muted-foreground"
          >
            {group.label}
          </h2>
          <ul>
            {group.requests.map((request) => (
              <li key={request.id} className="border-t">
                <ChatRow request={request} everyone={everyone} />
              </li>
            ))}
          </ul>
        </section>
      ))}
    </div>
  )
}

function ChatRow({
  request,
  everyone,
}: {
  request: RequestSummary
  everyone: boolean
}) {
  const state = chatState(request.state)
  return (
    <Link
      to="/c/$requestId"
      params={{ requestId: request.id }}
      className={rowLinkClass}
    >
      <RowText
        title={request.title}
        meta={
          <>
            <StatusLabel tone={state.tone}>{state.word}</StatusLabel>
            <span>{chatOutcome(request)}</span>
          </>
        }
      />
      {everyone ? (
        <span className="flex flex-none items-center gap-1.5 text-xs text-muted-foreground">
          <AuthorMark
            mark={markOf({
              ...request.requested_by,
              origin: null,
              client: null,
              run_id: null,
            })}
            size={20}
          />
          <span className="max-w-[16ch] truncate">
            {request.requested_by.name}
          </span>
        </span>
      ) : null}
      <span className="flex-none text-xs text-muted-foreground">
        <RelativeTime iso={request.created_at} />
      </span>
    </Link>
  )
}

function ChatsSkeleton() {
  return (
    <div
      aria-busy="true"
      className="overflow-hidden rounded-card border bg-card"
    >
      {Array.from({ length: skeletonRows }, (_, index) => (
        <Skeleton
          key={index}
          className={cn("h-14 rounded-none", index > 0 && "border-t")}
        />
      ))}
    </div>
  )
}

function NextPage({
  failed,
  fetching,
  onLoad,
}: {
  failed: boolean
  fetching: boolean
  onLoad: () => unknown
}) {
  const sentinel = useRef<HTMLDivElement>(null)
  const { inView } = useInView(sentinel)
  useEffect(() => {
    if (inView && !fetching && !failed) onLoad()
  }, [inView, fetching, failed, onLoad])
  return (
    <div ref={sentinel} className="mt-3 flex justify-center">
      <Button
        variant="outline"
        disabled={fetching}
        aria-busy={fetching}
        onClick={() => void onLoad()}
      >
        Load more
      </Button>
    </div>
  )
}
