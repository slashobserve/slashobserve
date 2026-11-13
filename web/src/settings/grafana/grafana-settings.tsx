import { useState } from "react"
import { useQuery } from "@tanstack/react-query"

import { Skeleton } from "@/components/ui/skeleton"
import { SettingsCard, SettingsProblem } from "@/settings/settings-card"

import { ConnectionForm } from "./connection-form"
import { grafanaProblemText } from "./connection-problems"
import { ConnectionsCard } from "./connections-card"
import { ImportLauncher } from "./import-launcher"
import { ImportProgress } from "./import-progress"
import {
  connectionsQuery,
  useCreateConnection,
  type GrafanaConnection,
} from "./queries"

type Running = { connectionId: string; syncId: string }

type Props = {
  running: Running | null
  onStarted: (connectionId: string, syncId: string) => void
}

const formHint =
  "A Grafana URL and a service account token with the Viewer role on the folders to import."

export function GrafanaSettings({ running, onStarted }: Props) {
  const query = useQuery(connectionsQuery())
  const create = useCreateConnection()
  const [launching, setLaunching] = useState<GrafanaConnection | null>(null)

  if (query.isError && query.data === undefined) {
    return (
      <SettingsCard title="Connections" hint={formHint}>
        <SettingsProblem
          title="Could not load the Grafana connections"
          detail={grafanaProblemText(query.error)}
          retrying={query.isFetching}
          onRetry={() => void query.refetch()}
        />
      </SettingsCard>
    )
  }
  if (query.data === undefined) {
    return <Skeleton className="h-24 w-full" />
  }

  async function add(input: Parameters<typeof create.mutateAsync>[0]) {
    try {
      await create.mutateAsync(input)
      return true
    } catch {
      return false
    }
  }

  return (
    <>
      {query.data.length > 0 ? (
        <ConnectionsCard connections={query.data} onImport={setLaunching} />
      ) : null}
      {launching === null ? null : (
        <ImportLauncher
          key={launching.id}
          connection={launching}
          onCancel={() => setLaunching(null)}
          onStarted={(sync) => {
            setLaunching(null)
            onStarted(sync.connection_id, sync.id)
          }}
        />
      )}
      {running === null || launching !== null ? null : (
        <ImportProgress
          key={running.syncId}
          connectionId={running.connectionId}
          syncId={running.syncId}
        />
      )}
      <SettingsCard title="Add a connection" hint={formHint}>
        <ConnectionForm
          pending={create.isPending}
          serverError={create.isError ? grafanaProblemText(create.error) : null}
          onSubmit={add}
        />
      </SettingsCard>
    </>
  )
}
