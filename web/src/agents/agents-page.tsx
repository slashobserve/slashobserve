import { useRef } from "react"
import { useQuery } from "@tanstack/react-query"
import { PlusIcon } from "lucide-react"

import { ApiError } from "@/api/errors"
import { LoadProblem } from "@/components/load-problem"
import { PageHeader } from "@/components/page-header"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { sentence } from "@/observe/queries"

import { ConnectPanel } from "./connect-panel"
import { ConnectedTable } from "./connected-table"
import { ObserveAgentCard } from "./observe-agent-card"
import { agentsQuery } from "./queries"
import { ServiceAccountsTable } from "./service-accounts-table"

type Props = {
  connecting: boolean
  onConnectingChange: (connecting: boolean) => void
}

function isSignedInOnly(error: unknown): boolean {
  return (
    error instanceof ApiError &&
    error.status === 403 &&
    error.code === "user_session_required"
  )
}

function SectionSkeleton() {
  return <Skeleton className="h-28 rounded-card" />
}

export function AgentsPage({ connecting, onConnectingChange }: Props) {
  const query = useQuery(agentsQuery())
  const connectButton = useRef<HTMLButtonElement>(null)
  const agents = query.data

  function body() {
    if (agents !== undefined) {
      return (
        <>
          <ObserveAgentCard agent={agents.observe_agent} />
          <ConnectedTable
            rows={agents.connected}
            onConnect={() => onConnectingChange(true)}
          />
          <ServiceAccountsTable rows={agents.service_accounts} />
        </>
      )
    }
    if (isSignedInOnly(query.error)) {
      return (
        <p className="text-[13px] text-muted-foreground">
          The Agents page is for signed-in people.
        </p>
      )
    }
    if (query.isError) {
      return (
        <LoadProblem
          title="Could not load the agents"
          detail={sentence(query.error.message)}
          retrying={query.isFetching}
          onRetry={() => void query.refetch()}
        />
      )
    }
    return (
      <div aria-busy="true" className="flex flex-col gap-6">
        <SectionSkeleton />
        <SectionSkeleton />
        <SectionSkeleton />
      </div>
    )
  }

  return (
    <div className="mx-auto w-full max-w-6xl">
      <PageHeader
        title="Agents"
        actions={
          <Button ref={connectButton} onClick={() => onConnectingChange(true)}>
            <PlusIcon aria-hidden="true" />
            Connect an agent
          </Button>
        }
      />
      <div className="flex flex-col gap-6">{body()}</div>
      {connecting ? (
        <ConnectPanel
          onClose={() => onConnectingChange(false)}
          trigger={connectButton}
        />
      ) : null}
    </div>
  )
}
