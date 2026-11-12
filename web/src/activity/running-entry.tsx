import { Link, useRouteContext } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"

import { RunningFor } from "@/chat/run-duration"
import { StepTimeline } from "@/chat/step-timeline"
import { buildStatusText } from "@/observe/build"
import { isTerminal, observeRequestQuery } from "@/observe/queries"
import { CancelRequestButton } from "@/observe/request-actions"
import { useSettledRefresh } from "@/observe/use-settled-refresh"

import { boxClass, EntryFrame } from "./entry-frame"
import { currentStep, type TimelineEntry } from "./timeline-model"

type Props = {
  entry: Extract<TimelineEntry, { kind: "running" }>
}

export function RunningLinkContent({ stepText }: { stepText: string }) {
  return (
    <>
      <svg
        width={13}
        height={13}
        viewBox="0 0 24 24"
        fill="none"
        strokeWidth={2.4}
        strokeLinecap="round"
        aria-hidden="true"
        className="flex-none stroke-foreground motion-safe:animate-spin"
      >
        <path d="M21 12a9 9 0 1 1-6.2-8.6" />
      </svg>
      <span className="min-w-0 flex-1 wrap-anywhere" aria-live="polite">
        {stepText}
      </span>
      <span className="flex-none text-xs text-muted-foreground">
        Details
        <span className="sr-only">
          {" "}
          of the running agent request, opens in a new tab
        </span>
      </span>
    </>
  )
}

export function RunningEntry({ entry }: Props) {
  const { request } = entry
  const { principal } = useRouteContext({ from: "/_authed" })
  const { data } = useQuery(observeRequestQuery(request.id))
  const steps = data?.steps ?? []
  const last = steps.at(-1)
  const status = data?.status ?? request.status
  const canceling = data?.state === "canceling"
  useSettledRefresh({
    active: !isTerminal(data),
    status,
    dashboardId: request.dashboard_id,
  })
  const watchable = data?.build != null && data.requested_by.id === principal.id
  const stepText =
    data?.build != null || status === "queued" || canceling
      ? buildStatusText({
          status,
          build: data?.build ?? null,
          failureCode: data?.failure_code ?? null,
          queuePosition: data?.queue_position ?? null,
          canceling,
        })
      : last === undefined
        ? "Starting"
        : `${last.message}, step ${currentStep(steps)}`

  return (
    <EntryFrame
      avatar={{ kind: "agent" }}
      who="Agent"
      verb="is working on it"
      at={entry.at}
    >
      <div
        className={`${boxClass} flex items-center gap-2 border-dashed hover:bg-muted`}
      >
        <Link
          to="/c/$requestId"
          params={{ requestId: request.id }}
          target="_blank"
          rel="noopener"
          className="flex min-w-0 flex-1 items-center gap-2 rounded-control no-underline outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <RunningLinkContent stepText={stepText} />
        </Link>
        {watchable ? (
          <Link
            from="/d/$dashboardId/{-$slug}"
            to="."
            search={(prev) => ({ ...prev, chat: request.id })}
            className="flex-none rounded-control text-xs text-muted-foreground outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
          >
            Open chat
          </Link>
        ) : null}
      </div>
      <StepTimeline
        steps={steps}
        working={status === "running"}
        reconnecting={false}
        latest={3}
        liveText={data?.live_text}
      />
      {status === "running" ? (
        <RunningFor startedAt={data?.started_at ?? request.started_at} />
      ) : null}
      {data !== undefined ? (
        <div className="self-start">
          <CancelRequestButton request={data} />
        </div>
      ) : null}
    </EntryFrame>
  )
}
