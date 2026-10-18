import { useQueryClient } from "@tanstack/react-query"
import { Link, useNavigate } from "@tanstack/react-router"
import { CheckIcon, Loader2Icon, XIcon } from "lucide-react"
import { useEffect, useLayoutEffect, useRef, useState } from "react"
import { toast } from "sonner"

import { ApiError } from "@/api/errors"
import { dashboardKeys } from "@/api/keys"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button, buttonVariants } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { cn } from "@/lib/utils"
import {
  describeSubmitError,
  isTerminal,
  sentence,
  stopsPolling,
  useCreateObserveRequest,
  useObserveRequest,
  usePollFailures,
  type ObserveRequest,
} from "@/observe/queries"
import {
  groupSteps,
  retryAction,
  type Step,
  type StepGroup,
} from "@/observe/request-model"

const reconnectingAfterFailures = 2

const phaseLabels: Record<Step["phase"], string> = {
  discover: "Discovering",
  lint: "Checking queries",
  query: "Running queries",
  save: "Saving",
  repair: "Repairing",
}

function formatElapsed(totalSeconds: number): string {
  if (totalSeconds < 60) return `${totalSeconds}s`
  const minutes = Math.floor(totalSeconds / 60)
  return `${minutes}m ${String(totalSeconds % 60).padStart(2, "0")}s`
}

function useElapsedSeconds(request: ObserveRequest | undefined): number {
  const [now, setNow] = useState(() => Date.now())
  const terminal = isTerminal(request)
  useEffect(() => {
    if (terminal) return
    const timer = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(timer)
  }, [terminal])
  if (request === undefined) return 0
  const end =
    terminal && request.finished_at !== null
      ? Date.parse(request.finished_at)
      : now
  return Math.max(0, Math.round((end - Date.parse(request.created_at)) / 1000))
}

function StepRow({ group }: { group: StepGroup }) {
  const { step, count } = group
  const failed = step.status === "error"
  return (
    <li className="flex items-start gap-3 px-3 py-2.5 text-sm">
      {failed ? (
        <XIcon
          aria-hidden
          className="mt-0.5 size-4 shrink-0 text-destructive"
        />
      ) : (
        <CheckIcon aria-hidden className="mt-0.5 size-4 shrink-0" />
      )}
      <div className="flex min-w-0 flex-1 flex-col gap-0.5 sm:flex-row sm:gap-4">
        <span className="font-medium sm:w-40 sm:shrink-0">
          {phaseLabels[step.phase]}
          {failed ? <span className="sr-only"> failed</span> : null}
        </span>
        <span className="min-w-0 wrap-anywhere text-muted-foreground">
          {count > 1 ? `${count} times, latest: ` : null}
          {step.message}
        </span>
      </div>
    </li>
  )
}

function StatusRow({ children }: { children: string }) {
  return (
    <li className="flex items-center gap-3 px-3 py-2.5 text-sm text-muted-foreground">
      <Loader2Icon
        aria-hidden
        className="size-4 shrink-0 animate-spin motion-reduce:animate-none"
      />
      <span>{children}</span>
    </li>
  )
}

function Steps({
  request,
  opening,
  stalled,
  reconnecting,
}: {
  request: ObserveRequest
  opening: boolean
  stalled: boolean
  reconnecting: boolean
}) {
  const working = !isTerminal(request) && !stalled
  return (
    <ol
      aria-live="polite"
      aria-relevant="additions"
      aria-label="Progress"
      className="divide-y rounded-lg border empty:hidden"
    >
      {groupSteps(request.steps).map((group, index) => (
        <StepRow key={index} group={group} />
      ))}
      {working ? <StatusRow>Working</StatusRow> : null}
      {working && reconnecting ? (
        <li className="py-2.5 pr-3 pl-10 text-sm text-muted-foreground">
          Reconnecting...
        </li>
      ) : null}
      {opening ? <StatusRow>Opening the dashboard</StatusRow> : null}
    </ol>
  )
}

function PromptHeading({ prompt }: { prompt: string }) {
  const heading = useRef<HTMLHeadingElement>(null)
  const [expanded, setExpanded] = useState(false)
  const [overflowing, setOverflowing] = useState(false)

  useEffect(() => {
    heading.current?.focus()
  }, [])

  useLayoutEffect(() => {
    const element = heading.current
    if (element === null || expanded) return
    const measure = () =>
      setOverflowing(element.scrollHeight > element.clientHeight + 1)
    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(element)
    return () => observer.disconnect()
  }, [prompt, expanded])

  return (
    <>
      <h1
        ref={heading}
        tabIndex={-1}
        className={cn(
          "text-2xl font-semibold wrap-anywhere whitespace-pre-line outline-none",
          !expanded && "line-clamp-3"
        )}
      >
        {prompt}
      </h1>
      {overflowing || expanded ? (
        <Button
          variant="link"
          size="sm"
          className="h-auto self-start border-0 p-0 text-sm underline"
          aria-expanded={expanded}
          onClick={() => setExpanded((value) => !value)}
        >
          {expanded ? "Show less" : "Show full prompt"}
        </Button>
      ) : null}
    </>
  )
}

function RetryButton({
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

function Failure({ request }: { request: ObserveRequest }) {
  const navigate = useNavigate()
  const create = useCreateObserveRequest()
  const [retrying, setRetrying] = useState(false)
  const action = retryAction(request)
  const dashboardId = action.kind === "create" ? null : action.dashboardId

  async function retry() {
    setRetrying(true)
    try {
      const next = await create.mutateAsync({
        prompt: request.prompt,
        dashboardId: action.kind === "revise" ? action.dashboardId : undefined,
      })
      await navigate({
        to: "/observe/$requestId",
        params: { requestId: next.id },
        replace: true,
      })
    } catch (cause) {
      toast.error(describeSubmitError(cause).message)
      setRetrying(false)
    }
  }

  return (
    <Alert variant="destructive">
      <AlertTitle>
        {request.source === "observe"
          ? "The dashboard could not be built"
          : "The dashboard could not be updated"}
      </AlertTitle>
      <AlertDescription>
        <p>{sentence(request.error ?? "The request failed")}</p>
        <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
          {action.kind !== "none" ? (
            <RetryButton busy={retrying} onClick={() => void retry()}>
              Try again
            </RetryButton>
          ) : null}
          {dashboardId !== null ? (
            <Link
              to="/dashboards/$dashboardId"
              params={{ dashboardId }}
              className="underline underline-offset-4"
            >
              Back to the dashboard
            </Link>
          ) : action.kind === "none" ? (
            <Link to="/" className="underline underline-offset-4">
              Back to dashboards
            </Link>
          ) : null}
        </div>
      </AlertDescription>
    </Alert>
  )
}

function Stalled({
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
      className="flex flex-wrap items-center justify-between gap-3 text-sm text-destructive"
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

function RequestNotFound({ removed }: { removed: boolean }) {
  return (
    <div className="flex min-h-full flex-col items-center justify-center gap-4 text-center">
      <h1 className="text-2xl font-semibold">Request not found</h1>
      <p className="text-sm text-muted-foreground">
        {removed
          ? "This request no longer exists."
          : "This request does not exist."}
      </p>
      <Link
        to="/dashboards"
        className={cn(buttonVariants({ variant: "outline" }))}
      >
        Back to dashboards
      </Link>
    </div>
  )
}

function ProgressSkeleton() {
  return (
    <div aria-busy className="mx-auto w-full max-w-5xl">
      <Skeleton className="h-8 w-2/3" />
      <Skeleton className="mt-1 h-5 w-24" />
      <Skeleton className="mt-6 h-24 w-full" />
    </div>
  )
}

export function RequestProgress({ requestId }: { requestId: string }) {
  const query = useObserveRequest(requestId)
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const request = query.data
  const elapsed = useElapsedSeconds(request)
  const running = request !== undefined && !isTerminal(request)
  const dashboardId =
    request?.status === "succeeded" ? request.dashboard_id : null
  const stalled = running && stopsPolling(query.error)
  const pollFailures = usePollFailures(requestId)
  const reconnecting =
    running && !stalled && pollFailures >= reconnectingAfterFailures

  useEffect(() => {
    if (!running) return
    const previous = document.title
    document.title = "Observing - SlashObserve"
    return () => {
      document.title = previous
    }
  }, [running])

  useEffect(() => {
    if (dashboardId === null) return
    void queryClient.invalidateQueries({ queryKey: dashboardKeys.list() })
    void navigate({
      to: "/dashboards/$dashboardId",
      params: { dashboardId },
      replace: true,
    })
  }, [dashboardId, navigate, queryClient])

  if (
    query.error instanceof ApiError &&
    (query.error.status === 404 ||
      (request === undefined && query.error.status === 400))
  ) {
    return <RequestNotFound removed={request !== undefined} />
  }

  if (request === undefined) {
    if (query.isError) {
      return (
        <div className="mx-auto w-full max-w-5xl">
          <Alert variant="destructive">
            <AlertTitle>Could not load this request</AlertTitle>
            <AlertDescription>
              <p>{sentence(query.error.message)}</p>
              <RetryButton
                busy={query.isFetching}
                onClick={() => void query.refetch()}
              >
                Retry
              </RetryButton>
            </AlertDescription>
          </Alert>
        </div>
      )
    }
    return <ProgressSkeleton />
  }

  return (
    <div className="mx-auto w-full max-w-5xl">
      <header className="mb-6 flex flex-col gap-1">
        <PromptHeading prompt={request.prompt} />
        <p className="text-sm text-muted-foreground tabular-nums">
          Elapsed {formatElapsed(elapsed)}
        </p>
      </header>
      <div className="flex flex-col gap-4">
        {request.status === "failed" ? <Failure request={request} /> : null}
        {request.status === "succeeded" && dashboardId === null ? (
          <p className="text-sm text-muted-foreground">
            The request finished without a dashboard.{" "}
            <Link to="/" className="underline underline-offset-4">
              Back to dashboards
            </Link>
          </p>
        ) : null}
        {stalled ? (
          <Stalled
            message={query.error?.message ?? ""}
            retrying={query.isFetching}
            onRetry={() => void query.refetch()}
          />
        ) : null}
        <Steps
          request={request}
          opening={dashboardId !== null}
          stalled={stalled}
          reconnecting={reconnecting}
        />
      </div>
    </div>
  )
}
