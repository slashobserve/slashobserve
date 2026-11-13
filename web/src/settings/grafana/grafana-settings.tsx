import { useRef, useState } from "react"
import { useQuery } from "@tanstack/react-query"
import { LockIcon, PlusIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { SettingsCard, SettingsProblem } from "@/settings/settings-card"
import { SettingsSection } from "@/settings/settings-section"
import type { SettingsPanel } from "@/settings/sections"

import { AddConnectionPanel, connectionsHint } from "./add-connection-panel"
import { grafanaProblemText } from "./connection-problems"
import { ConnectionsCard } from "./connections-card"
import { ImportLauncher } from "./import-launcher"
import { ImportProgress } from "./import-progress"
import { connectionsQuery, type GrafanaConnection } from "./queries"

type Running = { connectionId: string; syncId: string }

type Props = {
  running: Running | null
  onStarted: (connectionId: string, syncId: string) => void
  panelOpen: boolean
  onPanel: (panel: SettingsPanel | null) => void
}

export function GrafanaSettings({
  running,
  onStarted,
  panelOpen,
  onPanel,
}: Props) {
  const query = useQuery(connectionsQuery())
  const [launching, setLaunching] = useState<GrafanaConnection | null>(null)
  const addButton = useRef<HTMLButtonElement>(null)

  return (
    <SettingsSection
      title="Grafana connection"
      description="Connect a Grafana server and import its dashboards with their folders."
      action={
        <Button
          ref={addButton}
          variant={panelOpen ? "outline" : "default"}
          className="narrow:h-11"
          onClick={() => onPanel({ kind: "grafana-add" })}
        >
          <PlusIcon aria-hidden="true" />
          Add connection
        </Button>
      }
    >
      <SettingsCard title="Connections" hint={connectionsHint}>
        {query.isError && query.data === undefined ? (
          <SettingsProblem
            title="Could not load the Grafana connections"
            detail={grafanaProblemText(query.error)}
            retrying={query.isFetching}
            onRetry={() => void query.refetch()}
          />
        ) : query.data === undefined ? (
          <div className="border-t p-4">
            <Skeleton className="h-16 w-full" />
          </div>
        ) : query.data.length === 0 ? (
          <p className="border-t px-4 py-3 text-[13px] text-muted-foreground">
            No connections yet.
          </p>
        ) : (
          <ConnectionsCard connections={query.data} onImport={setLaunching} />
        )}
        <p className="flex items-center gap-2 border-t px-4 py-3 text-[13px] text-muted-foreground">
          <LockIcon aria-hidden="true" className="size-3.5 flex-none" />
          The token is stored encrypted and is never shown again.
        </p>
      </SettingsCard>
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
      {panelOpen ? (
        <AddConnectionPanel onClose={() => onPanel(null)} trigger={addButton} />
      ) : null}
    </SettingsSection>
  )
}
