import { useQuery, useQueryClient } from "@tanstack/react-query"
import { Link, useRouteContext } from "@tanstack/react-router"
import { useEffect, useMemo, useRef, useState } from "react"
import type { RefObject } from "react"

import { ApiError } from "@/api/errors"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Skeleton } from "@/components/ui/skeleton"
import { latestDashboardQuery } from "@/dashboards/queries"
import { useDocumentTitle } from "@/lib/document-title"
import { cn } from "@/lib/utils"
import { orderTurns } from "@/observe/conversation"
import {
  sentence,
  useAgentInfo,
  useObserveRequest,
  type ObserveRequest,
} from "@/observe/queries"
import { requestOrigin } from "@/observe/request-origin"
import { revisionsQuery } from "@/revisions/queries"

import { AskAgainButton } from "./ask-again"
import { ChatComposer, useOpenSent } from "./chat-composer"
import { useChat } from "./chats-queries"
import { RequestNotFound } from "./request-not-found"
import { RequestTurn, type TurnSummary } from "./request-turn"
import { RetryButton } from "./turn-failure"

const stickThreshold = 80
const stepsLoadedTurns = 3
const titlePromptLength = 60

function isActiveStatus(status: TurnSummary["status"]): boolean {
  return status === "queued" || status === "running"
}

function scrollParent(element: HTMLElement): HTMLElement | null {
  for (
    let parent = element.parentElement;
    parent !== null;
    parent = parent.parentElement
  ) {
    const overflow = getComputedStyle(parent).overflowY
    if (overflow === "auto" || overflow === "scroll") return parent
  }
  return null
}

function useChatScroll(
  list: RefObject<HTMLDivElement | null>,
  target: string | null
) {
  const pinned = useRef(false)
  const scroller = useRef<HTMLElement | null>(null)

  useEffect(() => {
    const element = list.current
    if (element === null || target === null) return
    const parent = scrollParent(element)
    if (parent === null) return
    scroller.current = parent
    const atEnd = () =>
      parent.scrollHeight - parent.scrollTop - parent.clientHeight <=
      stickThreshold
    let lastTop = parent.scrollTop
    const measure = () => {
      const top = parent.scrollTop
      if (atEnd()) pinned.current = true
      else if (top < lastTop) pinned.current = false
      lastTop = top
    }
    const section =
      target === "" ? null : document.getElementById(`request-${target}`)
    if (section === null) {
      parent.scrollTop = parent.scrollHeight
    } else {
      section.scrollIntoView({ block: "start" })
    }
    lastTop = parent.scrollTop
    pinned.current = atEnd()
    parent.addEventListener("scroll", measure, { passive: true })
    let lastTurn = element.lastElementChild
    let lastHeight = lastTurn?.getBoundingClientRect().height ?? 0
    const observer = new ResizeObserver(() => {
      const last = element.lastElementChild
      const height = last?.getBoundingClientRect().height ?? 0
      const grew = last !== lastTurn || height > lastHeight
      lastTurn = last
      lastHeight = height
      const busy = element.querySelector('[aria-busy="true"]') !== null
      if (pinned.current && (grew || busy)) {
        parent.scrollTop = parent.scrollHeight
      }
    })
    observer.observe(element)
    return () => {
      parent.removeEventListener("scroll", measure)
      observer.disconnect()
    }
  }, [list, target])

  return function pinToEnd() {
    pinned.current = true
    const parent = scroller.current
    if (parent !== null) parent.scrollTop = parent.scrollHeight
  }
}

function TurnSkeleton() {
  return (
    <div aria-busy="true" className="flex flex-col gap-[18px]">
      <Skeleton className="h-11 w-2/3 self-end rounded-xl" />
      <div className="flex gap-3">
        <Skeleton className="size-[26px] flex-none rounded-[7px]" />
        <Skeleton className="h-[172px] flex-1 rounded-card" />
      </div>
    </div>
  )
}

function ConversationSkeleton() {
  return (
    <div className="mx-auto w-full max-w-[760px] pt-2">
      <TurnSkeleton />
    </div>
  )
}

function LoadError({
  message,
  busy,
  onRetry,
}: {
  message: string
  busy: boolean
  onRetry: () => void
}) {
  return (
    <div className="mx-auto w-full max-w-[760px]">
      <Alert variant="destructive">
        <AlertTitle>Could not load this request</AlertTitle>
        <AlertDescription>
          <p>{sentence(message)}</p>
          <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
            <RetryButton busy={busy} onClick={onRetry}>
              Retry
            </RetryButton>
            <Link to="/ask" className="underline underline-offset-4">
              Back to Observe
            </Link>
          </div>
        </AlertDescription>
      </Alert>
    </div>
  )
}

type Turn = { id: string; summary: TurnSummary }

export type ConversationLayout = "page" | "pane"

function PageTitle({ title }: { title: string }) {
  useDocumentTitle(title)
  return null
}

function PageHeading({ heading }: { heading: string }) {
  const title = useRef<HTMLHeadingElement>(null)
  useEffect(() => {
    title.current?.focus({ preventScroll: true })
  }, [])
  return (
    <h1 ref={title} tabIndex={-1} className="sr-only outline-none">
      {heading}
    </h1>
  )
}

function PaneHeading({ requestId }: { requestId: string }) {
  return (
    <div className="flex w-full items-center justify-between gap-2 pb-2">
      <h2 className="text-sm font-semibold">Chat</h2>
      <Link
        to="/c/$requestId"
        params={{ requestId }}
        className="rounded-sm text-xs text-muted-foreground underline outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
      >
        Open the chat page
      </Link>
    </div>
  )
}

type ViewProps = {
  requestId: string
  layout: ConversationLayout
  dashboardId: string | null
  dashboardTitle: string | undefined
  turns: Turn[]
  ready: boolean
  historyFailed: boolean
  retryHistory: () => void
  historyBusy: boolean
  readOnly: boolean
  onSent: (request: ObserveRequest) => void
}

function ConversationView({
  requestId,
  layout,
  dashboardId,
  dashboardTitle,
  turns,
  ready,
  historyFailed,
  retryHistory,
  historyBusy,
  readOnly,
  onSent,
}: ViewProps) {
  const { principal } = useRouteContext({ from: "/_authed" })
  const agent = useAgentInfo()
  const list = useRef<HTMLDivElement>(null)
  const pane = layout === "pane"
  const [initialTarget, setInitialTarget] = useState<string | null>(null)
  if (ready && initialTarget === null) {
    setInitialTarget(turns.at(-1)?.id === requestId ? "" : requestId)
  }
  const pinToEnd = useChatScroll(list, initialTarget)

  const first = turns.at(0)
  const heading = dashboardTitle ?? first?.summary.prompt ?? ""
  const active = turns.some((turn) => isActiveStatus(turn.summary.status))
  const clipped =
    [...heading].length > titlePromptLength
      ? `${[...heading].slice(0, titlePromptLength).join("")}...`
      : heading
  const pageTitle = active
    ? "Observing - SlashObserve"
    : `${clipped} - SlashObserve`
  const runningRequestId = turns.findLast(
    (turn) =>
      isActiveStatus(turn.summary.status) &&
      turn.summary.requested_by.id === principal.id
  )?.id

  function appended(request: ObserveRequest) {
    pinToEnd()
    onSent(request)
  }

  const openSent = useOpenSent(appended)

  function focusList() {
    list.current?.focus({ preventScroll: true })
  }

  const agentOff = agent.data?.available === false
  const hasDashboard = dashboardId !== null
  const latestTurn = turns.at(-1)
  const waitingForAnswer =
    !hasDashboard &&
    latestTurn !== undefined &&
    isActiveStatus(latestTurn.summary.status)
  const loadFrom = Math.max(0, turns.length - stepsLoadedTurns)

  const transcript = (
    <>
      {pane ? (
        <PaneHeading requestId={requestId} />
      ) : (
        <>
          <PageTitle title={pageTitle} />
          <PageHeading heading={heading} />
        </>
      )}
      <div
        ref={list}
        role="region"
        aria-label="Conversation"
        tabIndex={-1}
        className={cn(
          "flex w-full flex-1 flex-col gap-[18px] pt-2 pb-6 outline-none",
          !pane && "max-w-[760px]"
        )}
      >
        {historyFailed ? (
          <div className="flex flex-wrap items-center gap-3 text-[13px] text-muted-foreground">
            <span>Earlier messages could not be loaded.</span>
            <RetryButton busy={historyBusy} onClick={retryHistory}>
              Retry
            </RetryButton>
          </div>
        ) : null}
        {initialTarget === null ? (
          <TurnSkeleton />
        ) : (
          turns.map((turn, index) => (
            <div
              key={turn.id}
              style={
                index === turns.length - 1
                  ? { viewTransitionName: "chat-prompt" }
                  : undefined
              }
            >
              <RequestTurn
                summary={turn.summary}
                dashboardTitle={dashboardTitle}
                loadSteps={index >= loadFrom || turn.id === requestId}
                onRetried={appended}
                buildOnScreen={pane && turn.id === requestId}
                opensRunsIn={pane ? "build" : "chat"}
                latest={index === turns.length - 1}
                answerActions={
                  !readOnly &&
                  index === turns.length - 1 &&
                  requestOrigin(turn.summary) === "observe" &&
                  turn.summary.requested_by.id === principal.id ? (
                    <AskAgainButton
                      prompt={turn.summary.prompt}
                      dashboardId={dashboardId ?? undefined}
                      followsId={hasDashboard ? undefined : turn.id}
                      disabled={active}
                      onSent={openSent}
                      afterSent={focusList}
                    />
                  ) : undefined
                }
              />
            </div>
          ))
        )}
      </div>
    </>
  )

  return (
    <div
      className={cn(
        "flex flex-col items-center",
        pane ? "min-h-0 flex-1" : "min-h-full"
      )}
    >
      {pane ? (
        <div className="flex min-h-0 w-full flex-1 flex-col overflow-y-auto">
          {transcript}
        </div>
      ) : (
        transcript
      )}
      {readOnly ? null : (
        <div
          style={{ viewTransitionName: "chat-composer" }}
          className={cn(
            "w-full bg-background pt-2 pb-1",
            pane ? "flex-none" : "sticky -bottom-7 max-w-[760px]"
          )}
        >
          <ChatComposer
            rows={2}
            disabled={waitingForAnswer}
            dashboardId={dashboardId ?? undefined}
            followsId={hasDashboard ? undefined : latestTurn?.id}
            dashboardTitle={dashboardTitle}
            runningRequestId={runningRequestId}
            placeholder={
              hasDashboard
                ? `Ask for another change to ${dashboardTitle ?? "this dashboard"}`
                : "Ask a follow-up, or describe a dashboard to build"
            }
            onSent={openSent}
          />
          {!agentOff ? (
            <p className="mt-1.5 text-center text-xs text-muted-foreground">
              {hasDashboard
                ? "Each change is saved as a new version of the dashboard."
                : waitingForAnswer
                  ? "Wait for the answer, or cancel the run, then ask a follow-up."
                  : "The agent sees the earlier messages of this chat."}
            </p>
          ) : null}
        </div>
      )}
    </div>
  )
}

export function Conversation({
  requestId,
  layout,
  asks = true,
  onSent,
}: {
  requestId: string
  layout: ConversationLayout
  asks?: boolean
  onSent?: (request: ObserveRequest) => void
}) {
  const queryClient = useQueryClient()
  const root = useObserveRequest(requestId)
  const [sent, setSent] = useState<ObserveRequest[]>([])
  const chatId = root.data?.chat_id ?? undefined
  const readsChat = chatId !== undefined
  const chat = useChat(chatId)
  const dashboardId =
    root.data?.dashboard_id ?? chat.data?.chat.dashboard_id ?? null
  const readOnly = root.data?.read_only === true || !asks
  const readsDashboard = dashboardId !== null && !readOnly
  const revisions = useQuery({
    ...revisionsQuery(dashboardId ?? ""),
    enabled: readsDashboard && !readsChat,
  })
  const latest = useQuery({
    ...latestDashboardQuery(queryClient, dashboardId ?? ""),
    enabled: readsDashboard,
  })

  const history = readsChat ? chat.data?.turns : revisions.data

  const turns = useMemo<Turn[]>(() => {
    if (root.data === undefined) return []
    const summaries = new Map<string, TurnSummary>()
    for (const request of sent) summaries.set(request.id, request)
    for (const turn of history ?? []) summaries.set(turn.id, turn)
    summaries.set(requestId, root.data)
    const known = [...summaries.values()]
    const ids = orderTurns(requestId, known)
    return ids.flatMap((id) => {
      const summary = summaries.get(id)
      return summary === undefined ? [] : [{ id, summary }]
    })
  }, [root.data, history, sent, requestId])

  if (
    root.error instanceof ApiError &&
    (root.error.status === 404 ||
      (root.data === undefined && root.error.status === 400))
  ) {
    return (
      <RequestNotFound
        removed={root.data !== undefined}
        pane={layout === "pane"}
      />
    )
  }

  if (root.data === undefined) {
    if (root.isError) {
      return (
        <LoadError
          message={root.error.message}
          busy={root.isFetching}
          onRetry={() => void root.refetch()}
        />
      )
    }
    return <ConversationSkeleton />
  }

  return (
    <ConversationView
      requestId={requestId}
      layout={layout}
      dashboardId={dashboardId}
      dashboardTitle={latest.data?.dashboard.title}
      turns={turns}
      readOnly={readOnly}
      ready={
        readsChat
          ? chat.data !== undefined || chat.isError
          : !readsDashboard || revisions.data !== undefined || revisions.isError
      }
      historyFailed={
        readsChat
          ? chat.isError && chat.data === undefined
          : revisions.isError && revisions.data === undefined
      }
      retryHistory={() =>
        void (readsChat ? chat.refetch() : revisions.refetch())
      }
      historyBusy={readsChat ? chat.isFetching : revisions.isFetching}
      onSent={(request) => {
        setSent((previous) => [...previous, request])
        onSent?.(request)
      }}
    />
  )
}
