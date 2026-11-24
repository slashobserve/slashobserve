import { Link, useNavigate } from "@tanstack/react-router"
import { useState } from "react"
import { toast } from "sonner"

import { StatusIcon } from "@/components/status-label"
import { Button, buttonVariants } from "@/components/ui/button"
import { cn } from "@/lib/utils"
import {
  describeSubmitError,
  sentence,
  useRetryRequest,
  type ObserveRequest,
} from "@/observe/queries"
import { dashboardLink } from "@/lib/links"
import {
  requestOrigin,
  type OriginFields,
  type RequestOrigin,
} from "@/observe/request-origin"

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

type FailedRequest = OriginFields &
  Pick<
    ObserveRequest,
    | "id"
    | "dashboard_id"
    | "error"
    | "failure_code"
    | "continued_by"
    | "retried_by"
    | "allowed_actions"
  >

export type RunOpens = "chat" | "build"

function newRunPage(
  request: OriginFields & Pick<ObserveRequest, "dashboard_id">,
  opens: RunOpens
): "/b/$requestId" | "/c/$requestId" | null {
  const origin = requestOrigin(request)
  if (origin === "observe") {
    return opens === "build" ? "/b/$requestId" : "/c/$requestId"
  }
  if (origin === "investigate" && request.dashboard_id === null) {
    return "/c/$requestId"
  }
  return null
}

export function useOpenNewRun(
  request: OriginFields & Pick<ObserveRequest, "dashboard_id">,
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

function NextRunNote({
  origin,
  runId,
  children,
}: {
  origin: RequestOrigin
  runId: string | null
  children: string
}) {
  if (runId === null) return null
  return origin === "observe" ? (
    <Link
      to="/c/$requestId"
      params={{ requestId: runId }}
      className="rounded-sm text-foreground underline outline-none focus-visible:ring-2 focus-visible:ring-ring"
    >
      {children}
    </Link>
  ) : (
    <span className="text-muted-foreground">{children}</span>
  )
}

function failureHeading(request: FailedRequest, buildLeft: boolean): string {
  const origin = requestOrigin(request)
  switch (origin) {
    case "observe":
      return request.intent === "ask" && !buildLeft
        ? "The question could not be answered"
        : "The dashboard could not be built"
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
  const retryRequest = useRetryRequest(request.id)
  const openNewRun = useOpenNewRun(request, onRetried, opensRunsIn)
  const buildLeft =
    request.allowed_actions.includes("continue") ||
    request.allowed_actions.includes("keep_build")
  const explained = failureText(request.failure_code, buildLeft)
  const detail = request.error === null ? null : sentence(request.error)
  const detailRepeatsMeaning =
    explained !== null &&
    detail?.toLowerCase() === explained.meaning.toLowerCase()
  const [retrying, setRetrying] = useState(false)
  const canRetry = request.allowed_actions.includes("retry")
  const followedUp =
    request.continued_by !== null || request.retried_by !== null
  const stopped = request.failure_code === "canceled"

  async function retry() {
    setRetrying(true)
    try {
      await openNewRun(await retryRequest.mutateAsync())
    } catch (cause) {
      toast.error(describeSubmitError(cause).message)
    } finally {
      setRetrying(false)
    }
  }

  return (
    <div
      role={stopped ? "status" : "alert"}
      className="flex items-start gap-2.5 rounded-card border px-3.5 py-3 text-[13px]"
    >
      {stopped ? null : (
        <span className="flex h-[1.45em] items-center">
          <StatusIcon tone="critical" />
        </span>
      )}
      <div className="flex min-w-0 flex-1 flex-col gap-2.5">
        <div className="flex flex-col gap-0.5">
          {stopped ? (
            <p className="font-semibold">You stopped this run</p>
          ) : (
            <>
              <p className="font-semibold">
                {failureHeading(request, buildLeft)}
              </p>
              <p className="wrap-anywhere text-muted-foreground">
                {explained?.meaning ??
                  sentence(request.error ?? "The request failed")}
              </p>
              {explained !== null &&
              detail !== null &&
              !detailRepeatsMeaning ? (
                <p className="wrap-anywhere text-muted-foreground">{detail}</p>
              ) : null}
            </>
          )}
          {explained !== null ? <p>{explained.next}</p> : null}
        </div>
        {followedUp || canRetry || request.dashboard_id !== null ? (
          <div className="flex flex-wrap items-center gap-2">
            <NextRunNote
              origin={requestOrigin(request)}
              runId={request.continued_by}
            >
              Continued in a new run
            </NextRunNote>
            <NextRunNote
              origin={requestOrigin(request)}
              runId={request.retried_by}
            >
              Tried again in a new run
            </NextRunNote>
            {canRetry ? (
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
