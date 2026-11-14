import { useQuery, useQueryClient } from "@tanstack/react-query"
import { Link, useRouteContext } from "@tanstack/react-router"
import { useEffect, useMemo, useRef, useState } from "react"
import type { RefObject } from "react"

import { ApiError } from "@/api/errors"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { buttonVariants } from "@/components/ui/button"
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
import { revisionsQuery } from "@/revisions/queries"

import { ChatComposer, useOpenSent } from "./chat-composer"
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

function RequestNotFound({ removed }: { removed: boolean }) {
  return (
    <div className="flex min-h-full flex-col items-center justify-center gap-4 text-center">
      <h1 className="text-2xl font-semibold">Request not found</h1>
      <p className="text-sm text-muted-foreground">
        {removed
          ? "This request no longer exists."
          : "This request does not exist."}
      </p>
      <Link to="/ask" className={cn(buttonVariants({ variant: "outline" }))}>
        Back to Observe
      </Link>
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

  const root = turns.find((turn) => turn.id === requestId)
  const heading = dashboardTitle ?? root?.summary.prompt ?? ""
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

  const openSent = useOpenSent(dashboardId === null ? undefined : appended)

  const agentOff = agent.data?.available === false
  const hasDashboard = dashboardId !== null
  const waitingForDashboard =
    !hasDashboard && root !== undefined && isActiveStatus(root.summary.status)
  const loadFrom = Math.max(0, turns.length - stepsLoadedTurns)

  return (
    <div className="flex min-h-full flex-col items-center">
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
        className={cn(
          "flex w-full flex-1 flex-col gap-[18px] pt-2 pb-6",
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
              />
            </div>
          ))
        )}
      </div>
      {readOnly ? null : (
        <div
          style={{ viewTransitionName: "chat-composer" }}
          className={cn(
            "sticky w-full bg-background pt-2 pb-1",
            pane ? "bottom-0" : "-bottom-7 max-w-[760px]"
          )}
        >
          <ChatComposer
            rows={2}
            disabled={waitingForDashboard}
            dashboardId={dashboardId ?? undefined}
            dashboardTitle={dashboardTitle}
            runningRequestId={runningRequestId}
            placeholder={
              hasDashboard
                ? `Ask for another change to ${dashboardTitle ?? "this dashboard"}`
                : "Describe a dashboard, like: checkout health by pod, last 6h"
            }
            onSent={openSent}
          />
          {!agentOff && (hasDashboard || waitingForDashboard) ? (
            <p className="mt-1.5 text-center text-xs text-muted-foreground">
              {hasDashboard
                ? "Each change is saved as a new version of the dashboard."
                : "Wait for the dashboard, then ask for changes."}
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
}: {
  requestId: string
  layout: ConversationLayout
}) {
  const queryClient = useQueryClient()
  const root = useObserveRequest(requestId)
  const [sent, setSent] = useState<ObserveRequest[]>([])
  const dashboardId = root.data?.dashboard_id ?? null
  const readOnly = root.data?.read_only === true
  const readsDashboard = dashboardId !== null && !readOnly
  const revisions = useQuery({
    ...revisionsQuery(dashboardId ?? ""),
    enabled: readsDashboard,
  })
  const latest = useQuery({
    ...latestDashboardQuery(queryClient, dashboardId ?? ""),
    enabled: readsDashboard,
  })

  const turns = useMemo<Turn[]>(() => {
    if (root.data === undefined) return []
    const summaries = new Map<string, TurnSummary>()
    for (const request of sent) summaries.set(request.id, request)
    for (const revision of revisions.data ?? []) {
      summaries.set(revision.id, revision)
    }
    summaries.set(requestId, root.data)
    const known = [...summaries.values()]
    const ids = orderTurns(requestId, dashboardId === null ? undefined : known)
    return ids.flatMap((id) => {
      const summary = summaries.get(id)
      return summary === undefined ? [] : [{ id, summary }]
    })
  }, [root.data, revisions.data, sent, requestId, dashboardId])

  if (
    root.error instanceof ApiError &&
    (root.error.status === 404 ||
      (root.data === undefined && root.error.status === 400))
  ) {
    return <RequestNotFound removed={root.data !== undefined} />
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
        !readsDashboard || revisions.data !== undefined || revisions.isError
      }
      historyFailed={revisions.isError && revisions.data === undefined}
      retryHistory={() => void revisions.refetch()}
      historyBusy={revisions.isFetching}
      onSent={(request) => setSent((previous) => [...previous, request])}
    />
  )
}
