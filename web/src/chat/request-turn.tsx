import { useQuery } from "@tanstack/react-query"
import { useRouteContext } from "@tanstack/react-router"
import { useState } from "react"

import { AgentMark } from "@/components/agent-mark"
import { StatusIcon } from "@/components/status-label"
import { buildStatusText, type BuildProgress } from "@/observe/build"
import { costText, shortRequestId, stepSummary } from "@/observe/conversation"
import {
  isTerminal,
  observeRequestQuery,
  type ObserveRequest,
  stopsPolling,
  usePollFailures,
} from "@/observe/queries"
import { CancelRequestButton } from "@/observe/request-actions"
import { useSettledRefresh } from "@/observe/use-settled-refresh"

import { FindingCard } from "@/findings/finding-card"

import { BuildCard } from "./build-card"
import { ResultCard } from "./result-card"
import { RunningFor } from "./run-duration"
import { StepTimeline } from "./step-timeline"
import {
  RetryButton,
  Stalled,
  TurnFailure,
  useOpenNewRun,
  type RunOpens,
} from "./turn-failure"
import {
  mayHaveFinding,
  resultFinishedAt,
  showsWorkingRow,
  tookText,
} from "./turn-model"

const reconnectingAfterFailures = 2

export type TurnSummary = {
  id: string
  source: "observe" | "prompt" | "comment" | "investigate"
  prompt: string
  status: "queued" | "running" | "succeeded" | "failed"
  state: ObserveRequest["state"]
  result_kind: ObserveRequest["result_kind"]
  allowed_actions: ObserveRequest["allowed_actions"]
  dashboard_id: string | null
  result_version: number | null
  error: string | null
  panel_id: string | null
  requested_by: { id: string; name: string }
  created_at: string
  started_at: string | null
  finished_at: string | null
  failure_code: string | null
  build: BuildProgress | null
  build_saved_at: string | null
  continues_id: string | null
  continued_by: string | null
  continue_note: string | null
  from?: string | null
  to?: string | null
}

type Props = {
  summary: TurnSummary
  dashboardTitle?: string
  loadSteps: boolean
  onRetried: (request: ObserveRequest) => void
  buildOnScreen?: boolean
  opensRunsIn?: RunOpens
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

function ContinuedFrom({ summary }: { summary: TurnSummary }) {
  if (summary.continues_id === null) return null
  return (
    <>
      <p className="self-end text-xs text-muted-foreground">
        Continues an earlier run
      </p>
      {summary.continue_note !== null ? (
        <p className="max-w-[80%] self-end text-xs wrap-anywhere whitespace-pre-wrap text-muted-foreground">
          {summary.continue_note}
        </p>
      ) : null}
    </>
  )
}

export function RequestTurn({
  summary,
  dashboardTitle,
  loadSteps,
  onRetried,
  buildOnScreen = false,
  opensRunsIn = "chat",
}: Props) {
  const { principal } = useRouteContext({ from: "/_authed" })
  const [detailsOpen, setDetailsOpen] = useState(false)
  const investigates = summary.source === "investigate"
  const query = useQuery({
    ...observeRequestQuery(summary.id),
    enabled:
      isActive(summary.status) ||
      loadSteps ||
      detailsOpen ||
      mayHaveFinding(summary),
  })
  const live = query.data
  const settled = !isActive(summary.status)
  const showLive = live !== undefined && !(settled && !isTerminal(live))
  const request: TurnSummary = showLive ? live : summary
  const detail = showLive ? live : undefined
  const build = request.build
  const answers = mayHaveFinding(request)
  const openNewRun = useOpenNewRun(request, onRetried, opensRunsIn)
  const isRequester = request.requested_by.id === principal.id
  const active = isActive(request.status)
  const stalled = active && stopsPolling(query.error)
  const pollFailures = usePollFailures(summary.id)
  const reconnecting =
    active && !stalled && pollFailures >= reconnectingAfterFailures
  const steps = live?.steps ?? []
  const savedBuild = request.result_kind === "partial_version"
  const inPane = opensRunsIn === "build"

  useSettledRefresh({
    active,
    status: request.status,
    dashboardId: request.dashboard_id,
  })

  return (
    <section
      id={`request-${summary.id}`}
      aria-label={`Request ${shortRequestId(summary.id)}`}
      aria-busy={active}
      className="flex scroll-mt-4 flex-col gap-[18px]"
    >
      <div className="flex flex-col gap-1.5">
        <Sender summary={request} />
        <ContinuedFrom summary={request} />
        <div className="max-w-[80%] self-end rounded-xl border px-3.5 py-2.5 text-sm wrap-anywhere whitespace-pre-wrap">
          {request.prompt}
        </div>
      </div>
      <div className="flex gap-3">
        <AgentMark variant="tile" />
        <div className="flex min-w-0 flex-1 flex-col gap-3 pt-[3px]">
          {active ? (
            <div className="flex flex-col gap-2.5">
              {build !== null && !investigates ? (
                <BuildCard
                  request={detail ?? request}
                  build={build}
                  isRequester={isRequester}
                  onContinued={(next) => void openNewRun(next)}
                  compact={buildOnScreen}
                />
              ) : (
                <span className="flex items-center gap-2 text-[13px]">
                  <StatusIcon tone="running" />
                  {detail !== undefined &&
                  (detail.queue_position !== null ||
                    detail.state === "canceling")
                    ? buildStatusText({
                        status: detail.status,
                        build: null,
                        failureCode: detail.failure_code,
                        queuePosition: detail.queue_position,
                        canceling: detail.state === "canceling",
                      })
                    : investigates
                      ? "Investigating"
                      : dashboardTitle !== undefined
                        ? `Revising ${dashboardTitle}`
                        : request.dashboard_id !== null
                          ? "Revising the dashboard"
                          : "Building the dashboard"}
                </span>
              )}
              {detail?.can_cancel ? (
                <div className="self-start">
                  <CancelRequestButton request={detail} />
                </div>
              ) : null}
              <StepTimeline
                steps={steps}
                working={showsWorkingRow(request, stalled)}
                reconnecting={reconnecting}
                liveText={detail?.live_text}
                requestId={summary.id}
                pane={inPane}
              />
              {request.status === "running" ? (
                <RunningFor startedAt={request.started_at} />
              ) : null}
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
                {live === undefined
                  ? "Show steps"
                  : stepSummary(
                      live.steps,
                      tookText(request.started_at, request.finished_at),
                      costText(live.usage?.cost_usd)
                    )}
              </summary>
              <div className="flex flex-col gap-2.5 pt-2.5 pl-0.5">
                {live !== undefined ? (
                  <StepTimeline
                    steps={steps}
                    working={false}
                    reconnecting={false}
                    requestId={summary.id}
                    pane={inPane}
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
          {savedBuild ? (
            <p className="text-[13px]">
              You saved what was built as version {request.result_version}.
            </p>
          ) : null}
          {answers && request.status === "succeeded" ? (
            live?.finding != null ? (
              <FindingCard finding={live.finding} showOpenLink />
            ) : live !== undefined ? (
              <p className="text-[13px] text-muted-foreground">
                {investigates
                  ? "The run finished without a finding."
                  : "The request finished without a dashboard."}
              </p>
            ) : null
          ) : null}
          {!answers &&
          (request.status === "succeeded" || savedBuild) &&
          request.result_version !== null &&
          request.dashboard_id !== null ? (
            <ResultCard
              dashboardId={request.dashboard_id}
              version={request.result_version}
              finishedAt={resultFinishedAt(request)}
              eager={loadSteps}
            />
          ) : null}
          {request.status === "failed" && !savedBuild ? (
            <TurnFailure
              request={request}
              onRetried={onRetried}
              opensRunsIn={opensRunsIn}
            />
          ) : null}
          {request.status === "failed" && !savedBuild && build !== null ? (
            <BuildCard
              request={request}
              build={build}
              isRequester={isRequester}
              onContinued={(next) => void openNewRun(next)}
              compact={buildOnScreen}
            />
          ) : null}
        </div>
      </div>
    </section>
  )
}
