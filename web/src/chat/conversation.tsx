import { useQuery, useQueryClient } from "@tanstack/react-query"
import { Link, useNavigate } from "@tanstack/react-router"
import { useEffect, useMemo, useRef, useState } from "react"
import type { RefObject } from "react"

import { ApiError } from "@/api/errors"
import { dashboardKeys } from "@/api/keys"
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

import { ChatComposer } from "./chat-composer"
import { RequestTurn, type TurnSummary } from "./request-turn"
import { RetryButton } from "./turn-failure"

const stickThreshold = 80
const stepsLoadedTurns = 3
const titlePromptLength = 60

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
    const measure = () => {
      pinned.current =
        parent.scrollHeight - parent.scrollTop - parent.clientHeight <=
        stickThreshold
    }
    const section =
      target === "" ? null : document.getElementById(`request-${target}`)
    if (section === null) {
      parent.scrollTop = parent.scrollHeight
    } else {
      section.scrollIntoView({ block: "start" })
    }
    measure()
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
      <Link to="/" className={cn(buttonVariants({ variant: "outline" }))}>
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
            <Link to="/" className="underline underline-offset-4">
              Back to Observe
            </Link>
          </div>
        </AlertDescription>
      </Alert>
    </div>
  )
}

type Turn = { id: string; summary: TurnSummary }

type ViewProps = {
  requestId: string
  dashboardId: string | null
  dashboardTitle: string | undefined
  turns: Turn[]
  ready: boolean
  historyFailed: boolean
  retryHistory: () => void
  historyBusy: boolean
  onSent: (request: ObserveRequest) => void
}

function ConversationView({
  requestId,
  dashboardId,
  dashboardTitle,
  turns,
  ready,
  historyFailed,
  retryHistory,
  historyBusy,
  onSent,
}: ViewProps) {
  const navigate = useNavigate()
  const agent = useAgentInfo()
  const list = useRef<HTMLDivElement>(null)
  const [initialTarget, setInitialTarget] = useState<string | null>(null)
  if (ready && initialTarget === null) {
    setInitialTarget(turns.at(-1)?.id === requestId ? "" : requestId)
  }
  const pinToEnd = useChatScroll(list, initialTarget)

  const root = turns.find((turn) => turn.id === requestId)
  const heading = dashboardTitle ?? root?.summary.prompt ?? ""
  const active = turns.some(
    (turn) =>
      turn.summary.status === "queued" || turn.summary.status === "running"
  )
  const clipped =
    [...heading].length > titlePromptLength
      ? `${[...heading].slice(0, titlePromptLength).join("")}...`
      : heading
  useDocumentTitle(
    active ? "Observing - SlashObserve" : `${clipped} - SlashObserve`
  )

  const agentOff = agent.data?.available === false
  const hasDashboard = dashboardId !== null
  const loadFrom = Math.max(0, turns.length - stepsLoadedTurns)

  return (
    <div className="flex min-h-full flex-col items-center">
      <h1 className="sr-only">{heading}</h1>
      <div
        ref={list}
        className="flex w-full max-w-[760px] flex-1 flex-col gap-[18px] pt-2 pb-6"
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
            <RequestTurn
              key={turn.id}
              summary={turn.summary}
              dashboardTitle={dashboardTitle}
              loadSteps={index >= loadFrom || turn.id === requestId}
            />
          ))
        )}
      </div>
      <div className="sticky -bottom-7 w-full max-w-[760px] bg-background pt-2 pb-2">
        <ChatComposer
          rows={2}
          dashboardId={dashboardId ?? undefined}
          placeholder={
            hasDashboard
              ? `Ask for another change to ${dashboardTitle ?? "this dashboard"}`
              : "Describe a dashboard, like: checkout health by pod, last 6h"
          }
          onSent={(request) => {
            if (hasDashboard) {
              pinToEnd()
              onSent(request)
              return
            }
            return navigate({
              to: "/observe/$requestId",
              params: { requestId: request.id },
            })
          }}
        />
        {hasDashboard && !agentOff ? (
          <p className="mt-1.5 text-center text-xs text-muted-foreground">
            Each change is saved as a new version of the dashboard.
          </p>
        ) : null}
      </div>
    </div>
  )
}

export function Conversation({ requestId }: { requestId: string }) {
  const queryClient = useQueryClient()
  const root = useObserveRequest(requestId)
  const [sent, setSent] = useState<ObserveRequest[]>([])
  const dashboardId = root.data?.dashboard_id ?? null
  const revisions = useQuery({
    ...revisionsQuery(dashboardId ?? ""),
    enabled: dashboardId !== null,
  })
  const latest = useQuery({
    ...latestDashboardQuery(queryClient, dashboardId ?? ""),
    enabled: dashboardId !== null,
  })

  const rootSucceeded =
    root.data?.status === "succeeded" && dashboardId !== null
  useEffect(() => {
    if (rootSucceeded) {
      void queryClient.invalidateQueries({ queryKey: dashboardKeys.list() })
    }
  }, [rootSucceeded, queryClient])

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
      dashboardId={dashboardId}
      dashboardTitle={latest.data?.dashboard.title}
      turns={turns}
      ready={
        dashboardId === null ||
        revisions.data !== undefined ||
        revisions.isError
      }
      historyFailed={revisions.isError && revisions.data === undefined}
      retryHistory={() => void revisions.refetch()}
      historyBusy={revisions.isFetching}
      onSent={(request) => setSent((previous) => [...previous, request])}
    />
  )
}
