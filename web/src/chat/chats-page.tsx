import { useEffect, useMemo, useRef, useState, type RefObject } from "react"
import { Link, useRouteContext } from "@tanstack/react-router"
import { MessageSquareIcon, PinIcon, SearchIcon } from "lucide-react"

import { DataGrid, type DataGridColumn } from "@/components/data-grid/data-grid"
import { EmptyState } from "@/components/empty-state"
import { FilterToolbar } from "@/components/filter-toolbar"
import { LoadProblem } from "@/components/load-problem"
import { PageHeader } from "@/components/page-header"
import { RelativeTime } from "@/components/relative-time"
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
import { roleAtLeast } from "@/lib/roles"
import { cn } from "@/lib/utils"
import { loadProblemText } from "@/service-accounts/queries"

import { chatFooterText } from "./chat-footer"
import { ChatRenameForm, ChatRowMenu, chatRowId } from "./chat-row-menu"
import { dashboardChoices, dashboardLabel, noDashboardLabel } from "./chat-rows"
import { useChats, type Chat } from "./chats-queries"
import type { ChatsSearch } from "./chats-search"

type Props = {
  search: ChatsSearch
  onSearch: (next: ChatsSearch) => void
}

const pageSize = 30
const searchDelayMs = 300
const skeletonRows = 3
const anyDashboard = "any"
const mineSubtitle = "Only you and Admins can see your chats."
const everyoneSubtitle =
  "Everyone's chats with Observe Agent. Only admins see this view."

const scopeButtonClass =
  "h-6 rounded-[4px] border border-transparent px-2.5 text-[13px] text-muted-foreground outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring aria-pressed:border-border aria-pressed:bg-card aria-pressed:font-medium aria-pressed:text-foreground narrow:h-11"

export function ChatsPage({ search, onSearch }: Props) {
  const { principal } = useRouteContext({ from: "/_authed" })
  const admin = roleAtLeast(principal.role, "admin")
  const everyone = admin && search.scope === "all"
  const filtered = search.q !== undefined || search.dashboard !== undefined
  const { chats, query } = useChats(
    {
      q: search.q,
      dashboardId: search.dashboard,
      scope: everyone ? "all" : undefined,
    },
    pageSize
  )
  const footer = chatFooterText(chats.length, null, query.hasNextPage)

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
        <DashboardFilter
          chats={chats}
          value={search.dashboard}
          onChange={(dashboard) => onSearch({ ...search, dashboard })}
        />
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
      ) : chats.length === 0 ? (
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
            <EmptyState title="No chats yet" action={newChat}>
              Ask for a dashboard to start one.
            </EmptyState>
          )}
        </div>
      ) : (
        <>
          <ChatTable chats={chats} everyone={everyone} />
          {footer === null ? null : (
            <p className="mt-2 text-xs text-muted-foreground">{footer}</p>
          )}
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

function DashboardFilter({
  chats,
  value,
  onChange,
}: {
  chats: readonly Chat[]
  value: string | undefined
  onChange: (dashboard: string | undefined) => void
}) {
  const items = [
    { value: anyDashboard, label: "Dashboard: any" },
    ...dashboardChoices(chats, value).map((choice) => ({
      value: choice.id,
      label: `Dashboard: ${choice.label}`,
    })),
  ]
  return (
    <Select
      items={items}
      value={value ?? anyDashboard}
      onValueChange={(next) => {
        if (next === null) return
        onChange(next === anyDashboard ? undefined : next)
      }}
    >
      <SelectTrigger aria-label="Dashboard" className="w-56">
        <SelectValue />
      </SelectTrigger>
      <SelectContent alignItemWithTrigger={false} align="start">
        {items.map((item) => (
          <SelectItem key={item.value} value={item.value}>
            {item.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
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

const rowActionClass =
  "opacity-0 [.ag-row-hover_&]:opacity-100 [[data-narrow-row]:hover_&]:opacity-100 focus-visible:opacity-100 data-popup-open:opacity-100 [@media(hover:none)]:opacity-100"

function ChatTitle({
  chat,
  everyone,
  renaming,
  onRenamed,
}: {
  chat: Chat
  everyone: boolean
  renaming: boolean
  onRenamed: () => void
}) {
  if (renaming) return <ChatRenameForm request={chat} onDone={onRenamed} />
  return (
    <div className="flex min-w-0 flex-col gap-0.5 py-1.5">
      <Link
        id={chatRowId(chat.id)}
        to="/c/$requestId"
        params={{ requestId: chat.id }}
        className="flex min-w-0 items-center gap-1.5 rounded-sm font-medium outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring"
      >
        {chat.pinned_at !== null ? (
          <PinIcon
            aria-label="Pinned"
            className="size-3.5 flex-none text-muted-foreground"
          />
        ) : null}
        <span className="truncate">{chat.title}</span>
      </Link>
      {everyone ? (
        <span className="truncate text-xs text-muted-foreground">
          {chat.owner.name}
        </span>
      ) : null}
    </div>
  )
}

function chatColumns({
  everyone,
  afterDelete,
  renamingId,
  onRename,
}: {
  everyone: boolean
  afterDelete: RefObject<HTMLElement | null>
  renamingId: string | null
  onRename: (chatId: string | null) => void
}): DataGridColumn<Chat>[] {
  return [
    {
      id: "chat",
      header: "Chat",
      minWidth: 240,
      flex: 3,
      autoHeight: true,
      cell: (chat) => (
        <ChatTitle
          chat={chat}
          everyone={everyone}
          renaming={renamingId === chat.id}
          onRenamed={() => onRename(null)}
        />
      ),
    },
    {
      id: "dashboard",
      header: "Dashboard",
      minWidth: 160,
      flex: 2,
      cell: (chat) => (
        <span className="block max-w-[240px] truncate text-muted-foreground">
          {dashboardLabel(chat) ?? noDashboardLabel}
        </span>
      ),
    },
    {
      id: "turns",
      header: "Turns",
      width: 80,
      cell: (chat) => (
        <span className="text-muted-foreground">
          {chat.turns.toLocaleString()}
        </span>
      ),
    },
    {
      id: "lastMessage",
      header: "Last message",
      width: 140,
      cell: (chat) => (
        <span className="whitespace-nowrap text-muted-foreground">
          <RelativeTime iso={chat.last_turn_at} />
        </span>
      ),
    },
    {
      id: "actions",
      header: "Actions",
      headerCell: () => <span className="sr-only">Actions</span>,
      width: 56,
      action: true,
      cell: (chat) =>
        chat.read_only || renamingId === chat.id ? null : (
          <ChatRowMenu
            request={chat}
            wholeChat
            current={false}
            onRename={() => onRename(chat.id)}
            afterDelete={afterDelete}
            className={rowActionClass}
          />
        ),
    },
  ]
}

const chatId = (chat: Chat) => chat.id

function ChatTable({
  chats,
  everyone,
}: {
  chats: readonly Chat[]
  everyone: boolean
}) {
  const table = useRef<HTMLDivElement>(null)
  const [renamingId, setRenamingId] = useState<string | null>(null)
  const columns = useMemo(
    () =>
      chatColumns({
        everyone,
        afterDelete: table,
        renamingId,
        onRename: setRenamingId,
      }),
    [everyone, renamingId]
  )
  return (
    <DataGrid
      label="Chats"
      ref={table}
      rows={chats}
      columns={columns}
      rowId={chatId}
    />
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
          className={cn("h-11 rounded-none", index > 0 && "border-t")}
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
