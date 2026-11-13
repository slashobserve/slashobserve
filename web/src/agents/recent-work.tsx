import { useId } from "react"
import { useInfiniteQuery } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { agentKeys } from "@/api/keys"
import type { components } from "@/api/schema.gen"
import { LoadProblem } from "@/components/load-problem"
import { RelativeTime } from "@/components/relative-time"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { dashboardLink, observationLink } from "@/lib/links"
import { loadProblemText } from "@/service-accounts/queries"

import type { AgentRef } from "./agent-ref"

type AgentRun = components["schemas"]["AgentRun"]

const pageSize = 20
const skeletonRows = 2

const stateWords: Record<AgentRun["state"], string> = {
  queued: "Queued",
  running: "Running",
  canceling: "Stopping",
  succeeded: "Done",
  failed: "Failed",
  canceled: "Canceled",
}

function runsKey(agentRef: AgentRef) {
  const id = agentRef.kind === "observe" ? "" : agentRef.id
  return [...agentKeys.all, "runs", agentRef.kind, id] as const
}

function runsQuery(agentRef: AgentRef) {
  const kind = agentRef.kind === "observe" ? "builtin" : agentRef.kind
  const id = agentRef.kind === "observe" ? undefined : agentRef.id
  return {
    queryKey: runsKey(agentRef),
    initialPageParam: undefined as string | undefined,
    queryFn: ({
      pageParam,
      signal,
    }: {
      pageParam: string | undefined
      signal: AbortSignal
    }) =>
      api
        .GET("/agents/runs", {
          params: { query: { kind, id, limit: pageSize, cursor: pageParam } },
          signal,
        })
        .then(unwrap),
    getNextPageParam: (page: components["schemas"]["AgentRunPage"]) =>
      page.next_cursor ?? undefined,
  }
}

function RunRow({ run, showPerson }: { run: AgentRun; showPerson: boolean }) {
  return (
    <li className="flex flex-col gap-0.5 py-2.5">
      <div className="flex min-w-0 items-baseline justify-between gap-3">
        {run.openable ? (
          <Link
            to="/c/$requestId"
            params={{ requestId: run.id }}
            className="min-w-0 text-[13px] font-medium wrap-anywhere underline-offset-4 hover:underline"
          >
            {run.title}
          </Link>
        ) : (
          <span className="min-w-0 text-[13px] font-medium wrap-anywhere">
            {run.title}
          </span>
        )}
        <span className="shrink-0 text-[13px]">{stateWords[run.state]}</span>
      </div>
      <p className="flex flex-wrap gap-x-3 text-xs text-muted-foreground">
        <RelativeTime iso={run.created_at} />
        {showPerson ? <span>by {run.requested_by.name}</span> : null}
        {run.dashboard !== null ? (
          <Link
            {...dashboardLink(run.dashboard.id, run.dashboard.title)}
            className="underline underline-offset-4"
          >
            {run.dashboard.title}
          </Link>
        ) : null}
        {run.observation !== null ? (
          <Link
            {...observationLink(run.observation.id)}
            className="underline underline-offset-4"
          >
            {run.observation.title}
          </Link>
        ) : null}
      </p>
    </li>
  )
}

export function RecentWork({ agentRef }: { agentRef: AgentRef }) {
  const headingId = useId()
  const list = useInfiniteQuery(runsQuery(agentRef))
  const runs = list.data?.pages.flatMap((page) => page.items) ?? []
  const showPerson = list.data?.pages[0]?.scope === "workspace"

  let body
  if (list.isError && list.data === undefined) {
    body = (
      <LoadProblem
        title="Could not load recent work"
        detail={loadProblemText(list.error)}
        retrying={list.isFetching}
        onRetry={() => void list.refetch()}
      />
    )
  } else if (list.data === undefined) {
    body = (
      <div aria-busy="true" className="flex flex-col gap-2">
        {Array.from({ length: skeletonRows }, (_, index) => (
          <Skeleton key={index} className="h-10 rounded-card" />
        ))}
      </div>
    )
  } else if (runs.length === 0) {
    body = <p className="text-[13px] text-muted-foreground">No runs yet.</p>
  } else {
    body = (
      <>
        <ul className="flex flex-col divide-y">
          {runs.map((run) => (
            <RunRow key={run.id} run={run} showPerson={showPerson} />
          ))}
        </ul>
        {list.hasNextPage ? (
          <div className="flex">
            <Button
              variant="outline"
              disabled={list.isFetchingNextPage}
              aria-busy={list.isFetchingNextPage}
              onClick={() => void list.fetchNextPage()}
            >
              Load more
            </Button>
          </div>
        ) : null}
      </>
    )
  }

  return (
    <section aria-labelledby={headingId} className="flex flex-col gap-3">
      <h2 id={headingId} className="text-sm font-semibold">
        Recent work
      </h2>
      {body}
    </section>
  )
}
