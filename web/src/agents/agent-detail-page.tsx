import { type ReactNode } from "react"
import { useQuery } from "@tanstack/react-query"
import { Link, useRouteContext } from "@tanstack/react-router"

import { ApiError } from "@/api/errors"
import { AuthorMark } from "@/components/author-mark"
import { LoadProblem } from "@/components/load-problem"
import { RelativeTime } from "@/components/relative-time"
import { Badge } from "@/components/ui/badge"
import { buttonVariants } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { ModeSelect } from "@/connected-agents/mode-select"
import type { AuthorMarkModel } from "@/lib/author"
import { cn } from "@/lib/utils"
import { sentence } from "@/observe/queries"

import { findAgent, type AgentRef, type FoundAgent } from "./agent-ref"
import { PresenceStatus, grantMark } from "./connected-table"
import { observeAgentMark } from "./observe-agent-card"
import { agentsQuery } from "./queries"
import { accountMark } from "./service-accounts-table"
import { scopeLine, statCells, type Stats } from "./stats"

const columnClass = "flex w-full max-w-[720px] min-w-0 flex-col gap-6"

function NotFound() {
  return (
    <div className="flex min-h-full flex-col items-center justify-center gap-4 text-center">
      <h1 className="text-2xl font-semibold">Agent not found</h1>
      <p className="text-sm text-muted-foreground">
        This agent does not exist or is not yours to see.
      </p>
      <Link to="/agents" className={cn(buttonVariants({ variant: "outline" }))}>
        Back to Agents
      </Link>
    </div>
  )
}

function Skeletons() {
  return (
    <div aria-busy="true" className={columnClass}>
      <Skeleton className="h-8 w-1/2" />
      <Skeleton className="h-24 rounded-card" />
    </div>
  )
}

type Header = {
  mark: AuthorMarkModel
  name: string
  subtitle: string
  controls: ReactNode
  status: ReactNode
}

function Stat({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-0.5">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="text-lg font-semibold">{children}</dd>
    </div>
  )
}

function StatList({
  stats,
  extra,
}: {
  stats: Stats | null
  extra?: ReactNode
}) {
  const cells = statCells(stats)
  if (cells.length === 0 && extra === undefined) return null
  return (
    <dl className="grid grid-cols-2 gap-x-6 gap-y-3 @min-[560px]:grid-cols-4">
      {cells.map((cell) => (
        <Stat key={cell.label} label={cell.label}>
          {cell.value}
        </Stat>
      ))}
      {extra}
    </dl>
  )
}

function headerOf(found: FoundAgent, personName: string): Header {
  switch (found.kind) {
    case "observe":
      return {
        mark: observeAgentMark,
        name: "Observe Agent",
        subtitle: "Runs for anyone who asks",
        controls: null,
        status: (
          <Badge variant={found.row.enabled ? "default" : "outline"}>
            {found.row.enabled ? "On" : "Off"}
          </Badge>
        ),
      }
    case "grant":
      return {
        mark: grantMark(found.row),
        name: found.row.client_name,
        subtitle: `Acts for ${personName}`,
        controls: (
          <>
            <ModeSelect
              grantId={found.row.grant_id}
              mode={found.row.mode}
              name={found.row.client_name}
            />
            <Link
              to="/settings"
              hash="connected-agents"
              className="text-[13px] underline underline-offset-4"
            >
              Manage in Settings
            </Link>
          </>
        ),
        status: (
          <PresenceStatus
            present={found.row.present}
            lastSeenAt={found.row.last_seen_at}
          />
        ),
      }
    case "service_account":
      return {
        mark: accountMark(found.row),
        name: found.row.name,
        subtitle: "Service account",
        controls: null,
        status: (
          <PresenceStatus
            present={found.row.present}
            lastSeenAt={found.row.last_seen_at}
          />
        ),
      }
  }
}

function Details({ found }: { found: FoundAgent }) {
  switch (found.kind) {
    case "observe":
      return (
        <>
          <StatList stats={found.row.last_7_days} />
          <p className="text-xs text-muted-foreground">
            {scopeLine(found.row.scope)}
          </p>
        </>
      )
    case "grant":
      return (
        <StatList
          stats={found.row.last_7_days}
          extra={
            <>
              <Stat label="Writes in 7 days">{found.row.writes_7d}</Stat>
              <Stat label="Connected">
                <RelativeTime iso={found.row.connected_at} />
              </Stat>
            </>
          }
        />
      )
    case "service_account":
      return <StatList stats={found.row.last_7_days} />
  }
}

function LoadedAgent({ found }: { found: FoundAgent }) {
  const { principal } = useRouteContext({ from: "/_authed" })
  const header = headerOf(found, principal.name)
  return (
    <div className={columnClass}>
      <header className="flex flex-wrap items-center justify-between gap-x-6 gap-y-3">
        <div className="flex min-w-0 items-center gap-3">
          <AuthorMark mark={header.mark} />
          <div className="flex min-w-0 flex-col gap-0.5">
            <h1 className="text-xl font-semibold wrap-anywhere">
              {header.name}
            </h1>
            <p className="text-[13px] text-muted-foreground">
              {header.subtitle}
            </p>
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-3">
          {header.controls}
          <span className="text-[13px] text-muted-foreground">
            {header.status}
          </span>
        </div>
      </header>
      <Details found={found} />
    </div>
  )
}

export function AgentDetailPage({ agentRef }: { agentRef: AgentRef | null }) {
  const query = useQuery({ ...agentsQuery(), enabled: agentRef !== null })
  if (agentRef === null) return <NotFound />
  if (query.data !== undefined) {
    const found = findAgent(query.data, agentRef)
    return found === null ? <NotFound /> : <LoadedAgent found={found} />
  }
  if (query.error instanceof ApiError && query.error.status === 403) {
    return (
      <p className="text-[13px] text-muted-foreground">
        The Agents page is for signed-in people.
      </p>
    )
  }
  if (query.isError) {
    return (
      <div className={columnClass}>
        <LoadProblem
          title="Could not load this agent"
          detail={sentence(query.error.message)}
          retrying={query.isFetching}
          onRetry={() => void query.refetch()}
        />
      </div>
    )
  }
  return <Skeletons />
}
