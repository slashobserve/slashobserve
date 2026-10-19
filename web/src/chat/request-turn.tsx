import { useQuery, useQueryClient } from "@tanstack/react-query"
import { Link, useRouteContext } from "@tanstack/react-router"
import { useEffect, useRef, useState } from "react"

import { dashboardKeys } from "@/api/keys"
import { AgentMark } from "@/components/agent-mark"
import { shortRequestId, stepSummary } from "@/observe/conversation"
import {
  isTerminal,
  observeRequestQuery,
  type ObserveRequest,
  stopsPolling,
  usePollFailures,
} from "@/observe/queries"

import { ResultCard } from "./result-card"
import { StepList, ToneIcon } from "./step-list"
import { RetryButton, Stalled, TurnFailure } from "./turn-failure"

const reconnectingAfterFailures = 2

export type TurnSummary = {
  id: string
  source: "observe" | "prompt" | "comment"
  prompt: string
  status: "queued" | "running" | "succeeded" | "failed"
  dashboard_id: string | null
  result_version: number | null
  error: string | null
  panel_id: string | null
  requested_by: { id: string; name: string }
  created_at: string
  finished_at: string | null
}

type Props = {
  summary: TurnSummary
  dashboardTitle?: string
  loadSteps: boolean
  onRetried: (request: ObserveRequest) => void
}

function isActive(status: TurnSummary["status"]): boolean {
  return status === "queued" || status === "running"
}

function Sender({ summary }: { summary: TurnSummary }) {
  const { principal } = useRouteContext({ from: "/_authed" })
  const own = summary.requested_by.id === principal.id
  const fromDashboard =
    summary.source === "comment" || summary.panel_id !== null
  if (own && !fromDashboard) return null
  return (
    <p className="self-end text-xs text-muted-foreground">
      {own
        ? "Sent from the dashboard"
        : `Sent by ${summary.requested_by.name}${fromDashboard ? " from the dashboard" : ""}`}
    </p>
  )
}

export function RequestTurn({
  summary,
  dashboardTitle,
  loadSteps,
  onRetried,
}: Props) {
  const queryClient = useQueryClient()
  const [detailsOpen, setDetailsOpen] = useState(false)
  const query = useQuery({
    ...observeRequestQuery(summary.id),
    enabled: isActive(summary.status) || loadSteps || detailsOpen,
  })
  const live = query.data
  const settled = !isActive(summary.status)
  const request: TurnSummary =
    live !== undefined && !(settled && !isTerminal(live)) ? live : summary
  const active = isActive(request.status)
  const stalled = active && stopsPolling(query.error)
  const pollFailures = usePollFailures(summary.id)
  const reconnecting =
    active && !stalled && pollFailures >= reconnectingAfterFailures
  const steps = live?.steps ?? []

  const wasActive = useRef(active)
  const dashboardId = request.dashboard_id
  const status = request.status
  useEffect(() => {
    if (wasActive.current && !active) {
      if (status === "succeeded") {
        void queryClient.invalidateQueries({ queryKey: dashboardKeys.list() })
      }
      if (dashboardId !== null) {
        void queryClient.invalidateQueries({
          queryKey: dashboardKeys.revisions(dashboardId),
        })
        void queryClient.invalidateQueries({
          queryKey: dashboardKeys.latest(dashboardId),
        })
      }
    }
    wasActive.current = active
  }, [active, status, dashboardId, queryClient])

  return (
    <section
      id={`request-${summary.id}`}
      aria-label={`Request ${shortRequestId(summary.id)}`}
      aria-busy={active}
      className="flex scroll-mt-4 flex-col gap-[18px]"
    >
      <div className="flex flex-col gap-1.5">
        <Sender summary={request} />
        <div className="max-w-[80%] self-end rounded-xl border px-3.5 py-2.5 text-sm wrap-anywhere whitespace-pre-wrap">
          {request.prompt}
        </div>
      </div>
      <div className="flex gap-3">
        <AgentMark variant="tile" />
        <div className="flex min-w-0 flex-1 flex-col gap-3 pt-[3px]">
          {active ? (
            <div className="flex flex-col gap-2.5">
              <span className="flex items-center gap-2 text-[13px]">
                <ToneIcon tone="running" />
                {dashboardTitle !== undefined
                  ? `Revising ${dashboardTitle}`
                  : request.dashboard_id !== null
                    ? "Revising the dashboard"
                    : "Building the dashboard"}
              </span>
              <StepList
                steps={steps}
                working={!stalled}
                reconnecting={reconnecting}
              />
              {stalled ? (
                <Stalled
                  message={query.error?.message ?? ""}
                  retrying={query.isFetching}
                  onRetry={() => void query.refetch()}
                />
              ) : null}
            </div>
          ) : (
            <details
              onToggle={(event) => setDetailsOpen(event.currentTarget.open)}
            >
              <summary className="cursor-pointer text-[13px] text-muted-foreground">
                {live === undefined ? "Show steps" : stepSummary(live.steps)}
              </summary>
              <div className="pt-2.5 pl-0.5">
                {live !== undefined ? (
                  <StepList
                    steps={steps}
                    working={false}
                    reconnecting={false}
                  />
                ) : query.isError ? (
                  <div className="flex flex-wrap items-center gap-3 text-[13px] text-muted-foreground">
                    <span>Steps could not be loaded.</span>
                    <RetryButton
                      busy={query.isFetching}
                      onClick={() => void query.refetch()}
                    >
                      Retry
                    </RetryButton>
                  </div>
                ) : (
                  <p className="text-[13px] text-muted-foreground">
                    Loading steps...
                  </p>
                )}
              </div>
            </details>
          )}
          {request.status === "succeeded" &&
          request.result_version !== null &&
          request.dashboard_id !== null ? (
            <ResultCard
              dashboardId={request.dashboard_id}
              version={request.result_version}
              finishedAt={request.finished_at}
            />
          ) : null}
          {request.status === "succeeded" &&
          (request.result_version === null || request.dashboard_id === null) ? (
            <p className="text-[13px] text-muted-foreground">
              The request finished without a dashboard.
            </p>
          ) : null}
          {request.status === "failed" ? (
            <TurnFailure request={request} onRetried={onRetried} />
          ) : null}
          <Link
            to="/observe/$requestId"
            params={{ requestId: summary.id }}
            className="self-start rounded-control text-xs text-muted-foreground outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
          >
            Request {shortRequestId(summary.id)}
          </Link>
        </div>
      </div>
    </section>
  )
}
