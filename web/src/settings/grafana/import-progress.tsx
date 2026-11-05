import type { ReactNode } from "react"
import { Link } from "@tanstack/react-router"

import { ApiError } from "@/api/errors"
import { StatusLabel, type StatusTone } from "@/components/status-label"
import { Skeleton } from "@/components/ui/skeleton"
import { dashboardLink } from "@/lib/links"
import { SettingsCard, SettingsProblem } from "@/settings/settings-card"

import { grafanaProblemText } from "./connection-problems"
import { useImportSync, type GrafanaImport } from "./queries"
import { groupResults, type GrafanaImportResult } from "./results"

type Props = { connectionId: string; syncId: string }

const title = "Import progress"
const hint = "The server keeps the last 20 imports until it restarts."

const statuses: Record<
  GrafanaImport["status"],
  { tone: StatusTone; label: string }
> = {
  running: { tone: "running", label: "Importing" },
  finished: { tone: "ok", label: "Finished" },
  failed: { tone: "critical", label: "Failed" },
  canceled: { tone: "warning", label: "Stopped" },
}

export function ImportProgress({ connectionId, syncId }: Props) {
  const query = useImportSync(connectionId, syncId)

  if (query.isError && query.data === undefined) {
    const forgotten =
      query.error instanceof ApiError && query.error.status === 404
    return (
      <SettingsCard title={title} hint={hint}>
        {forgotten ? (
          <p className="px-4 py-3 text-[13px] text-muted-foreground">
            This import is no longer tracked, because the server restarted or
            ran 20 newer imports. The connection shows the last finished one.
          </p>
        ) : (
          <SettingsProblem
            title="Could not load the import"
            detail={grafanaProblemText(query.error)}
            retrying={query.isFetching}
            onRetry={() => void query.refetch()}
          />
        )}
      </SettingsCard>
    )
  }
  const sync = query.data
  if (sync === undefined) {
    return (
      <SettingsCard title={title} hint={hint}>
        <div className="p-4" aria-busy="true">
          <Skeleton className="h-4 w-48" />
        </div>
      </SettingsCard>
    )
  }
  return (
    <SettingsCard title={title} hint={hint}>
      <Progress sync={sync} />
      <Results results={sync.results} />
    </SettingsCard>
  )
}

function Progress({ sync }: { sync: GrafanaImport }) {
  const status = statuses[sync.status]
  return (
    <div
      role="status"
      className="flex flex-wrap items-center gap-x-4 gap-y-1 border-b px-4 py-3 text-[13px]"
    >
      <StatusLabel tone={status.tone}>{status.label}</StatusLabel>
      <span>
        {sync.total === 0 && sync.status === "running"
          ? "Listing dashboards"
          : `${sync.done} of ${sync.total} dashboards`}
      </span>
      <span className="text-muted-foreground">
        {sync.failed} failed, {sync.skipped} skipped
      </span>
      {sync.error === null ? null : (
        <span className="text-destructive">{sync.error}</span>
      )}
    </div>
  )
}

function Group({
  label,
  results,
  open,
  children,
}: {
  label: string
  results: GrafanaImportResult[]
  open: boolean
  children: (result: GrafanaImportResult) => ReactNode
}) {
  if (results.length === 0) return null
  const summary = `${label} (${results.length})`
  const list = (
    <ul className="flex flex-col gap-1.5 pt-1.5">
      {results.map((result) => (
        <li key={result.uid} className="text-[13px]">
          {children(result)}
        </li>
      ))}
    </ul>
  )
  return (
    <div className="border-b px-4 py-3 last:border-b-0">
      {open ? (
        <>
          <h3 className="text-[13px] font-medium">{summary}</h3>
          {list}
        </>
      ) : (
        <details>
          <summary className="cursor-pointer text-[13px] font-medium">
            {summary}
          </summary>
          {list}
        </details>
      )}
    </div>
  )
}

function Uid({ uid }: { uid: string }) {
  return <span className="font-mono text-xs text-muted-foreground">{uid}</span>
}

function DashboardTitle({ result }: { result: GrafanaImportResult }) {
  if (result.dashboard_id === null) return <>{result.title}</>
  return (
    <Link
      {...dashboardLink(result.dashboard_id, result.title)}
      className="underline-offset-2 hover:underline"
    >
      {result.title}
    </Link>
  )
}

function Results({ results }: { results: GrafanaImportResult[] }) {
  const groups = groupResults(results)
  return (
    <>
      <Group label="Failed" results={groups.failed} open>
        {(result) => (
          <div className="flex flex-wrap items-baseline gap-x-2">
            <span className="wrap-anywhere">{result.title}</span>
            <Uid uid={result.uid} />
            <span className="text-destructive">{result.error}</span>
          </div>
        )}
      </Group>
      <Group label="Need repair" results={groups.needsRepair} open>
        {(result) => (
          <div className="flex flex-wrap items-baseline gap-x-2">
            <DashboardTitle result={result} />
            <span className="text-muted-foreground">
              {result.unverified_panels} unverified, {result.placeholder_panels}{" "}
              placeholders
            </span>
          </div>
        )}
      </Group>
      {groups.needsRepair.length === 0 ? null : (
        <div className="border-b px-4 py-3 text-[13px]">
          <Link to="/import/repair" className="underline underline-offset-2">
            Open the repair queue
          </Link>
        </div>
      )}
      <Group label="Imported" results={groups.imported} open={false}>
        {(result) => <DashboardTitle result={result} />}
      </Group>
      <Group label="Skipped" results={groups.skipped} open={false}>
        {(result) => (
          <div className="flex flex-wrap items-baseline gap-x-2">
            <DashboardTitle result={result} />
            <Uid uid={result.uid} />
          </div>
        )}
      </Group>
    </>
  )
}
