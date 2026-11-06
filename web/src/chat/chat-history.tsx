import { useEffect, useId, useRef, useState, type RefObject } from "react"
import { useNavigate, useRouteContext } from "@tanstack/react-router"
import { BookmarkPlusIcon, HistoryIcon } from "lucide-react"

import { RelativeTime } from "@/components/relative-time"
import { StatusLabel } from "@/components/status-label"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { cn } from "@/lib/utils"
import { buildStatusText } from "@/observe/build"
import {
  requestStatusView,
  useMyRequests,
  type RequestSummary,
} from "@/observe/queries"
import { canSavePrompts } from "@/prompts/access"
import { usePromptPanel } from "@/prompts/prompt-panel-store"
import { SidePanel } from "@/shell/side-panel"
import { TopRowActions } from "@/shell/top-row-actions"

import { groupChats } from "./chat-list"
import { ChatRenameForm, ChatRowMenu, chatRowId } from "./chat-row-menu"
import { useHistoryPanel, type HistoryTab } from "./history-store"
import { ImportHistory } from "./import-history"
import { canSaveAsPrompt, savePromptButtonId } from "./save-prompt-target"

const pageSize = 20
const searchDelayMs = 250

const triggerClass =
  "h-auto flex-none rounded-none border-0 border-b-2 border-transparent px-0 pb-2 text-[13px] font-medium text-muted-foreground -mb-px after:hidden data-active:border-foreground data-active:text-foreground dark:data-active:border-foreground dark:group-data-[variant=line]/tabs-list:data-active:border-foreground"

export function ChatHistory({
  currentRequestId,
}: {
  currentRequestId?: string
}) {
  const open = useHistoryPanel((state) => state.open)
  const setOpen = useHistoryPanel((state) => state.setOpen)
  const tab = useHistoryPanel((state) => state.tab)
  const setTab = useHistoryPanel((state) => state.setTab)
  const saving = usePromptPanel((state) => state.target !== null)
  const panelId = useId()
  const trigger = useRef<HTMLButtonElement>(null)
  const heading = useRef<HTMLHeadingElement>(null)

  return (
    <>
      <TopRowActions>
        <Button
          ref={trigger}
          variant="ghost"
          size="sm"
          className="h-[30px] gap-1.5 text-[13px]"
          aria-expanded={open}
          aria-controls={panelId}
          onClick={() => setOpen(!open)}
        >
          <HistoryIcon aria-hidden="true" />
          History
        </Button>
      </TopRowActions>
      {open ? (
        <SidePanel
          title="History"
          description="Your chats and imports, newest first."
          trigger={trigger}
          titleRef={heading}
          hidden={saving}
          onClose={() => setOpen(false)}
        >
          <Tabs
            value={tab}
            onValueChange={(value) => setTab(value as HistoryTab)}
          >
            <TabsList
              variant="line"
              activateOnFocus
              aria-label="History"
              className="mb-1 h-auto w-full justify-start gap-[18px] border-b p-0"
            >
              <TabsTrigger value="chats" className={triggerClass}>
                Chats
              </TabsTrigger>
              <TabsTrigger value="imports" className={triggerClass}>
                Imports
              </TabsTrigger>
            </TabsList>
            <TabsContent value="chats">
              <HistoryList
                panelId={panelId}
                heading={heading}
                currentRequestId={currentRequestId}
              />
            </TabsContent>
            <TabsContent value="imports">
              <ImportHistory panelId={panelId} heading={heading} />
            </TabsContent>
          </Tabs>
        </SidePanel>
      ) : null}
    </>
  )
}

function useSearchText(): [string, (text: string) => void, string | undefined] {
  const [text, setText] = useState("")
  const [q, setQ] = useState<string | undefined>(undefined)
  useEffect(() => {
    const timer = setTimeout(() => {
      const trimmed = text.trim()
      setQ(trimmed === "" ? undefined : trimmed)
    }, searchDelayMs)
    return () => clearTimeout(timer)
  }, [text])
  return [text, setText, q]
}

function HistoryList({
  panelId,
  heading,
  currentRequestId,
}: {
  panelId: string
  heading: RefObject<HTMLHeadingElement | null>
  currentRequestId?: string
}) {
  const [text, setText, q] = useSearchText()
  return (
    <div id={panelId} className="flex flex-col gap-3">
      <Input
        type="search"
        aria-label="Search chats"
        placeholder="Search chats"
        value={text}
        onChange={(event) => setText(event.target.value)}
      />
      <HistoryResults
        q={q}
        heading={heading}
        currentRequestId={currentRequestId}
      />
    </div>
  )
}

function HistoryResults({
  q,
  heading,
  currentRequestId,
}: {
  q: string | undefined
  heading: RefObject<HTMLHeadingElement | null>
  currentRequestId?: string
}) {
  const groupIdPrefix = useId()
  const { requests, query } = useMyRequests({ q }, pageSize)
  const loadMoreFailed = query.isFetchNextPageError
  const loadMoreErrorId = useId()
  const rows = useRef<HTMLDivElement>(null)
  const loadMoreButton = useRef<HTMLButtonElement>(null)
  const retryButton = useRef<HTMLButtonElement>(null)
  const focusFrom = useRef<number | null>(null)
  const rowCount = requests.length
  const fetching = query.isFetching

  useEffect(() => {
    const from = focusFrom.current
    if (fetching || from === null) return
    focusFrom.current = null
    const active = document.activeElement
    const onBody = !active || active === document.body
    const focusRow = (index: number) => {
      const buttons =
        rows.current?.querySelectorAll<HTMLButtonElement>("[data-chat-row]")
      buttons?.[index]?.focus()
    }
    if (rowCount > from) {
      if (!onBody && active !== loadMoreButton.current) return
      focusRow(from)
    } else if (onBody) {
      if (rowCount > 0) focusRow(rowCount - 1)
      else (retryButton.current ?? heading.current)?.focus()
    }
  }, [rowCount, fetching, heading])

  const retry = () => {
    focusFrom.current = 0
    void query.refetch()
  }

  const loadMore = () => {
    focusFrom.current = rowCount
    void query.fetchNextPage()
  }

  if (query.isPending) {
    return (
      <div aria-busy="true" className="flex flex-col gap-2">
        {Array.from({ length: 5 }, (_, index) => (
          <Skeleton key={index} className="h-14" />
        ))}
      </div>
    )
  }

  if (query.data === undefined) {
    return (
      <div className="flex flex-col items-start gap-3">
        <p className="text-[13px] text-muted-foreground">
          Could not load your requests.
        </p>
        <Button
          ref={retryButton}
          variant="outline"
          size="sm"
          disabled={query.isFetching}
          focusableWhenDisabled
          onClick={retry}
        >
          Retry
        </Button>
      </div>
    )
  }

  if (requests.length === 0) {
    return (
      <p className="text-[13px] text-muted-foreground">
        {q === undefined
          ? "No requests yet. Ask for a dashboard to start one."
          : `No chats match "${q}".`}
      </p>
    )
  }

  return (
    <>
      <div ref={rows} className="flex flex-col gap-3">
        {groupChats(requests).map((group) => {
          const headingId = `${groupIdPrefix}-${group.key}`
          return (
            <section key={group.key} aria-labelledby={headingId}>
              <h3
                id={headingId}
                className="px-0.5 pb-1 text-xs font-medium text-muted-foreground"
              >
                {group.label}
              </h3>
              <ul className="-mx-2 flex flex-col">
                {group.requests.map((request) => (
                  <HistoryRow
                    key={request.id}
                    request={request}
                    current={request.id === currentRequestId}
                    heading={heading}
                  />
                ))}
              </ul>
            </section>
          )
        })}
      </div>
      {query.hasNextPage ? (
        <div className="flex flex-wrap items-center justify-center gap-x-3 gap-y-2">
          {loadMoreFailed ? (
            <p
              id={loadMoreErrorId}
              role="alert"
              className="text-xs text-destructive"
            >
              Could not load more requests.
            </p>
          ) : null}
          <Button
            ref={loadMoreButton}
            variant="outline"
            size="sm"
            disabled={query.isFetchingNextPage}
            focusableWhenDisabled
            aria-busy={query.isFetchingNextPage}
            aria-describedby={loadMoreFailed ? loadMoreErrorId : undefined}
            onClick={loadMore}
          >
            Load more
          </Button>
        </div>
      ) : null}
    </>
  )
}

const rowActionClass =
  "opacity-0 group-hover/row:opacity-100 focus-visible:opacity-100 data-popup-open:opacity-100 [@media(hover:none)]:opacity-100"

function HistoryRow({
  request,
  current,
  heading,
}: {
  request: RequestSummary
  current: boolean
  heading: RefObject<HTMLHeadingElement | null>
}) {
  const navigate = useNavigate()
  const { principal } = useRouteContext({ from: "/_authed" })
  const openPanel = usePromptPanel((state) => state.openPanel)
  const [renaming, setRenaming] = useState(false)
  const titleId = `history-title-${request.id}`
  const savable = canSavePrompts(principal) && canSaveAsPrompt(request)

  if (renaming) {
    return (
      <li className="flex items-start">
        <ChatRenameForm request={request} onDone={() => setRenaming(false)} />
      </li>
    )
  }

  return (
    <li className="group/row relative flex items-start">
      <button
        type="button"
        id={chatRowId(request.id)}
        data-chat-row
        aria-current={current ? "true" : undefined}
        className={cn(
          "flex min-w-0 flex-1 flex-col gap-1 rounded-control px-2 py-2.5 text-left outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring aria-[current=true]:bg-muted",
          savable ? "pr-16" : "pr-9"
        )}
        onClick={() =>
          void navigate({
            to: "/c/$requestId",
            params: { requestId: request.id },
          })
        }
      >
        <span id={titleId} className="line-clamp-2 text-[13px] wrap-anywhere">
          {request.title}
        </span>
        <HistoryMeta request={request} />
      </button>
      <div className="absolute top-2 right-0 flex items-center">
        {savable ? (
          <Button
            id={savePromptButtonId(request.id)}
            variant="ghost"
            size="icon-sm"
            aria-label="Save prompt"
            aria-describedby={titleId}
            className={rowActionClass}
            onClick={() =>
              openPanel({
                kind: "create",
                body: request.prompt,
                returnFocusId: savePromptButtonId(request.id),
              })
            }
          >
            <BookmarkPlusIcon aria-hidden="true" />
          </Button>
        ) : null}
        <ChatRowMenu
          request={request}
          current={current}
          onRename={() => setRenaming(true)}
          afterDelete={heading}
          className={rowActionClass}
        />
      </div>
    </li>
  )
}

function HistoryMeta({ request }: { request: RequestSummary }) {
  const view = requestStatusView(request)
  const statusText =
    request.status === "succeeded"
      ? request.result_version !== null
        ? `Saved version ${request.result_version}`
        : "Finished"
      : request.status === "failed" &&
          (request.build !== null || request.continued_by !== null)
        ? buildStatusText({
            status: request.status,
            build: request.build,
            failureCode: request.failure_code,
            continued: request.continued_by !== null,
          })
        : view.text

  return (
    <span className="flex flex-wrap items-center gap-x-3 gap-y-0.5 text-xs text-muted-foreground">
      <StatusLabel tone={view.tone}>
        {request.status === "failed" ? (
          <span className="max-w-[260px] truncate" title={statusText}>
            {statusText}
          </span>
        ) : (
          statusText
        )}
      </StatusLabel>
      {request.dashboard_title !== null ? (
        <span className="max-w-[200px] truncate">
          {request.dashboard_title}
        </span>
      ) : null}
      <RelativeTime iso={request.created_at} />
    </span>
  )
}
