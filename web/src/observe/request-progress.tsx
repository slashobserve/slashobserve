import { useQueryClient } from "@tanstack/react-query"
import { Link, useNavigate } from "@tanstack/react-router"
import { CheckIcon, Loader2Icon, XIcon } from "lucide-react"
import { useEffect, useState } from "react"
import { toast } from "sonner"

import { ApiError } from "@/api/errors"
import { dashboardKeys } from "@/api/keys"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button, buttonVariants } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import {
  describeSubmitError,
  isTerminal,
  sentence,
  useCreateObserveRequest,
  useObserveRequest,
  type ObserveRequest,
} from "@/observe/queries"

type Step = ObserveRequest["steps"][number]

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

function StepRow({ step }: { step: Step }) {
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
          {step.message}
        </span>
      </div>
    </li>
  )
}

function WorkingRow() {
  return (
    <li className="flex items-center gap-3 px-3 py-2.5 text-sm text-muted-foreground">
      <Loader2Icon
        aria-hidden
        className="size-4 shrink-0 animate-spin motion-reduce:animate-none"
      />
      <span>Working</span>
    </li>
  )
}

function Steps({ request }: { request: ObserveRequest }) {
  const working = !isTerminal(request)
  return (
    <ol
      aria-live="polite"
      aria-label="Progress"
      className="divide-y rounded-lg border empty:hidden"
    >
      {request.steps.map((step, index) => (
        <StepRow key={index} step={step} />
      ))}
      {working ? <WorkingRow /> : null}
    </ol>
  )
}

function Failure({ request }: { request: ObserveRequest }) {
  const navigate = useNavigate()
  const create = useCreateObserveRequest()
  const [retrying, setRetrying] = useState(false)

  async function retry() {
    setRetrying(true)
    try {
      const next = await create.mutateAsync(request.prompt)
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
      <AlertTitle>The dashboard could not be built</AlertTitle>
      <AlertDescription>
        <p>{sentence(request.error ?? "The request failed.")}</p>
        <Button
          variant="outline"
          size="sm"
          className="text-foreground"
          disabled={retrying}
          aria-busy={retrying}
          onClick={() => void retry()}
        >
          Try again
        </Button>
      </AlertDescription>
    </Alert>
  )
}

function RequestNotFound() {
  return (
    <main className="flex flex-1 flex-col items-center justify-center gap-4 p-6 text-center">
      <h1 className="text-2xl font-semibold">Request not found</h1>
      <p className="text-sm text-muted-foreground">
        This request does not exist.
      </p>
      <Link to="/" className={buttonVariants({ variant: "outline" })}>
        Back to dashboards
      </Link>
    </main>
  )
}

function ProgressSkeleton() {
  return (
    <main aria-busy className="mx-auto w-full max-w-3xl p-6">
      <Skeleton className="h-8 w-2/3" />
      <Skeleton className="mt-1 h-5 w-24" />
      <Skeleton className="mt-6 h-24 w-full" />
    </main>
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

  if (request === undefined) {
    if (query.isError) {
      const error = query.error
      if (error instanceof ApiError && [400, 404].includes(error.status)) {
        return <RequestNotFound />
      }
      return (
        <main className="mx-auto w-full max-w-3xl p-6">
          <Alert variant="destructive">
            <AlertTitle>Could not load this request</AlertTitle>
            <AlertDescription>
              <p>{sentence(error.message)}</p>
              <Button
                variant="outline"
                size="sm"
                className="text-foreground"
                disabled={query.isFetching}
                aria-busy={query.isFetching}
                onClick={() => void query.refetch()}
              >
                Retry
              </Button>
            </AlertDescription>
          </Alert>
        </main>
      )
    }
    return <ProgressSkeleton />
  }

  return (
    <main className="mx-auto w-full max-w-3xl p-6">
      <header className="mb-6 flex flex-col gap-1">
        <h1 className="text-2xl font-semibold wrap-anywhere whitespace-pre-line">
          {request.prompt}
        </h1>
        <p className="text-sm text-muted-foreground tabular-nums">
          Elapsed {formatElapsed(elapsed)}
        </p>
      </header>
      <div className="flex flex-col gap-4">
        <Steps request={request} />
        {request.status === "failed" ? <Failure request={request} /> : null}
        {request.status === "succeeded" && dashboardId === null ? (
          <p className="text-sm text-muted-foreground">
            The request finished without a dashboard.{" "}
            <Link to="/" className="underline underline-offset-4">
              Back to dashboards
            </Link>
          </p>
        ) : null}
      </div>
    </main>
  )
}
