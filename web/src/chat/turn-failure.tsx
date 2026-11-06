import { Link, useNavigate } from "@tanstack/react-router"
import { useState } from "react"
import { toast } from "sonner"

import { StatusIcon } from "@/components/status-label"
import { Button, buttonVariants } from "@/components/ui/button"
import { cn } from "@/lib/utils"
import {
  describeSubmitError,
  sentence,
  useCreateObserveRequest,
  type ObserveRequest,
} from "@/observe/queries"
import { retryAction } from "@/observe/request-model"
import { dashboardLink } from "@/lib/links"

import { failureText } from "./failure-text"

export function RetryButton({
  busy,
  onClick,
  children,
}: {
  busy: boolean
  onClick: () => void
  children: string
}) {
  return (
    <Button
      variant="outline"
      size="sm"
      className="text-foreground"
      disabled={busy}
      aria-busy={busy}
      onClick={onClick}
    >
      {children}
    </Button>
  )
}

export function OpenDashboardLink({
  dashboardId,
  title,
  variant,
}: {
  dashboardId: string
  title?: string
  variant: "primary" | "outline"
}) {
  return (
    <Link
      {...dashboardLink(dashboardId, title)}
      className={cn(
        buttonVariants(
          variant === "primary" ? {} : { variant: "outline", size: "sm" }
        )
      )}
    >
      Open dashboard
    </Link>
  )
}

type FailedRequest = Pick<
  ObserveRequest,
  | "source"
  | "dashboard_id"
  | "panel_id"
  | "prompt"
  | "error"
  | "failure_code"
  | "continued_by"
> &
  Partial<Pick<ObserveRequest, "from" | "to">>

export type RunOpens = "chat" | "build"

function newRunPage(
  request: Pick<ObserveRequest, "source" | "dashboard_id">,
  opens: RunOpens
): "/b/$requestId" | "/c/$requestId" | null {
  if (request.source === "observe") {
    return opens === "build" ? "/b/$requestId" : "/c/$requestId"
  }
  if (request.source === "investigate" && request.dashboard_id === null) {
    return "/c/$requestId"
  }
  return null
}

export function useOpenNewRun(
  request: Pick<ObserveRequest, "source" | "dashboard_id">,
  onRevised: (request: ObserveRequest) => void,
  opens: RunOpens = "chat"
): (next: ObserveRequest) => Promise<void> {
  const navigate = useNavigate()
  const page = newRunPage(request, opens)
  return async (next) => {
    if (page !== null) {
      await navigate({
        to: page,
        params: { requestId: next.id },
        replace: true,
      })
    } else {
      onRevised(next)
    }
  }
}

function ContinuedNote({ request }: { request: FailedRequest }) {
  if (request.continued_by === null) return null
  return request.source === "observe" ? (
    <Link
      to="/c/$requestId"
      params={{ requestId: request.continued_by }}
      className="rounded-sm text-foreground underline outline-none focus-visible:ring-2 focus-visible:ring-ring"
    >
      Continued in a new run
    </Link>
  ) : (
    <span className="text-muted-foreground">Continued in a new run</span>
  )
}

function failureHeading(source: FailedRequest["source"]): string {
  switch (source) {
    case "observe":
      return "The dashboard could not be built"
    case "investigate":
      return "The question could not be answered"
    default:
      return "The dashboard could not be updated"
  }
}

export function TurnFailure({
  request,
  onRetried,
  opensRunsIn = "chat",
}: {
  request: FailedRequest
  onRetried: (request: ObserveRequest) => void
  opensRunsIn?: RunOpens
}) {
  const create = useCreateObserveRequest()
  const openNewRun = useOpenNewRun(request, onRetried, opensRunsIn)
  const explained = failureText(request.failure_code)
  const [retrying, setRetrying] = useState(false)
  const action = retryAction(request)
  const continued = request.continued_by !== null

  async function retry() {
    setRetrying(true)
    try {
      const next = await create.mutateAsync(
        action.kind === "investigate"
          ? {
              prompt: request.prompt,
              kind: "investigate",
              dashboardId: action.dashboardId,
              panelId: action.panelId,
              from: action.from,
              to: action.to,
            }
          : {
              prompt: request.prompt,
              dashboardId:
                action.kind === "revise" ? action.dashboardId : undefined,
            }
      )
      await openNewRun(next)
    } catch (cause) {
      toast.error(describeSubmitError(cause).message)
    } finally {
      setRetrying(false)
    }
  }

  return (
    <div
      role="alert"
      className="flex items-start gap-2.5 rounded-card border px-3.5 py-3 text-[13px]"
    >
      <span className="flex h-[1.45em] items-center">
        <StatusIcon tone="critical" />
      </span>
      <div className="flex min-w-0 flex-1 flex-col gap-2.5">
        <div className="flex flex-col gap-0.5">
          <p className="font-semibold">{failureHeading(request.source)}</p>
          <p className="wrap-anywhere text-muted-foreground">
            {explained?.meaning ??
              sentence(request.error ?? "The request failed")}
          </p>
          {explained !== null && request.error !== null ? (
            <p className="wrap-anywhere text-muted-foreground">
              {sentence(request.error)}
            </p>
          ) : null}
          {explained !== null ? <p>{explained.next}</p> : null}
        </div>
        {continued ||
        action.kind !== "none" ||
        request.dashboard_id !== null ? (
          <div className="flex flex-wrap items-center gap-2">
            <ContinuedNote request={request} />
            {!continued && action.kind !== "none" ? (
              <RetryButton busy={retrying} onClick={() => void retry()}>
                Try again
              </RetryButton>
            ) : null}
            {request.dashboard_id !== null ? (
              <OpenDashboardLink
                dashboardId={request.dashboard_id}
                variant="outline"
              />
            ) : null}
          </div>
        ) : null}
      </div>
    </div>
  )
}

export function Stalled({
  message,
  retrying,
  onRetry,
}: {
  message: string
  retrying: boolean
  onRetry: () => void
}) {
  return (
    <div
      role="alert"
      className="flex flex-wrap items-center justify-between gap-3 text-[13px] text-destructive"
    >
      <p className="min-w-0 wrap-anywhere">
        Progress stopped updating. {sentence(message)}
      </p>
      <RetryButton busy={retrying} onClick={onRetry}>
        Retry
      </RetryButton>
    </div>
  )
}
