import { useState, type RefObject } from "react"
import { useQuery } from "@tanstack/react-query"
import { LoaderCircleIcon } from "lucide-react"

import { instanceQuery } from "@/api/instance"
import { LoadProblem } from "@/components/load-problem"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { CodeSnippet } from "@/service-accounts/code-snippet"
import { loadProblemText } from "@/service-accounts/queries"
import { SidePanel } from "@/shell/side-panel"

import { connectClients, isNewConnection } from "./connect-clients"
import { agentsQuery, type AgentsOverview } from "./queries"

type Props = {
  onClose: () => void
  trigger?: RefObject<HTMLElement | null>
}

const pollMs = 5000

function grantIds(overview: AgentsOverview | undefined): string[] | undefined {
  return overview?.connected.map((row) => row.grant_id)
}

export function ConnectPanel({ onClose, trigger }: Props) {
  const client = connectClients[0]
  const instance = useQuery(instanceQuery())
  const [baseline, setBaseline] = useState<string[] | null>(null)
  const agents = useQuery({
    ...agentsQuery(),
    refetchInterval: (query) => {
      const ids = grantIds(query.state.data)
      return baseline !== null &&
        ids !== undefined &&
        isNewConnection(baseline, ids) !== null
        ? false
        : pollMs
    },
  })
  const ids = grantIds(agents.data)
  if (baseline === null && ids !== undefined) setBaseline(ids)
  const addedId =
    baseline === null || ids === undefined
      ? null
      : isNewConnection(baseline, ids)
  const added =
    addedId === null
      ? undefined
      : agents.data?.connected.find((row) => row.grant_id === addedId)

  return (
    <SidePanel
      title="Connect an agent"
      onClose={onClose}
      trigger={trigger}
      footer={
        added === undefined ? undefined : (
          <Button onClick={onClose}>Done</Button>
        )
      }
    >
      <p className="text-[13px] font-medium">{client.label}</p>
      {instance.isPending ? (
        <Skeleton className="h-20 w-full" />
      ) : instance.data === undefined ? (
        <LoadProblem
          title="Could not load the server address"
          detail={loadProblemText(instance.error)}
          retrying={instance.isFetching}
          onRetry={() => void instance.refetch()}
        />
      ) : (
        <>
          <CodeSnippet
            code={client.snippet(instance.data.mcp_url)}
            label={`${client.label} connect command`}
          />
          <p className="text-xs text-muted-foreground">
            Approve the connection in the browser window it opens.
          </p>
        </>
      )}
      <p
        role="status"
        className="flex items-center gap-2 text-[13px] text-muted-foreground"
      >
        {added === undefined ? (
          <>
            <LoaderCircleIcon
              aria-hidden="true"
              className="size-3.5 motion-safe:animate-spin"
            />
            Waiting for {client.label} to connect
          </>
        ) : (
          <>Connected: {added.client_name}</>
        )}
      </p>
    </SidePanel>
  )
}
