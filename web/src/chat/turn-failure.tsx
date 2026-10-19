import { Link, useNavigate } from "@tanstack/react-router"
import { ArrowUpRightIcon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"

import { Button, buttonVariants } from "@/components/ui/button"
import { cn } from "@/lib/utils"
import {
  describeSubmitError,
  sentence,
  useCreateObserveRequest,
  type ObserveRequest,
} from "@/observe/queries"
import { retryAction } from "@/observe/request-model"

import { ToneIcon } from "./step-list"

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

export function OpenDashboardLink({ dashboardId }: { dashboardId: string }) {
  return (
    <Link
      to="/dashboards/$dashboardId"
      params={{ dashboardId }}
      target="_blank"
      rel="noopener"
      className={cn(buttonVariants({ variant: "outline", size: "sm" }))}
    >
      Open dashboard
      <ArrowUpRightIcon aria-hidden="true" className="size-[13px]" />
      <span className="sr-only"> (opens in a new tab)</span>
    </Link>
  )
}

type FailedRequest = Pick<
  ObserveRequest,
  "source" | "dashboard_id" | "prompt" | "error"
>

export function TurnFailure({ request }: { request: FailedRequest }) {
  const navigate = useNavigate()
  const create = useCreateObserveRequest()
  const [retrying, setRetrying] = useState(false)
  const action = retryAction(request)

  async function retry() {
    setRetrying(true)
    try {
      const next = await create.mutateAsync({
        prompt: request.prompt,
        dashboardId: action.kind === "revise" ? action.dashboardId : undefined,
      })
      if (action.kind === "create") {
        await navigate({
          to: "/observe/$requestId",
          params: { requestId: next.id },
          replace: true,
        })
      }
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
        <ToneIcon tone="critical" />
      </span>
      <div className="flex min-w-0 flex-1 flex-col gap-2.5">
        <div className="flex flex-col gap-0.5">
          <p className="font-semibold">
            {request.source === "observe"
              ? "The dashboard could not be built"
              : "The dashboard could not be updated"}
          </p>
          <p className="wrap-anywhere text-muted-foreground">
            {sentence(request.error ?? "The request failed")}
          </p>
        </div>
        {action.kind !== "none" || request.dashboard_id !== null ? (
          <div className="flex flex-wrap items-center gap-2">
            {action.kind !== "none" ? (
              <RetryButton busy={retrying} onClick={() => void retry()}>
                Try again
              </RetryButton>
            ) : null}
            {request.dashboard_id !== null ? (
              <OpenDashboardLink dashboardId={request.dashboard_id} />
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
