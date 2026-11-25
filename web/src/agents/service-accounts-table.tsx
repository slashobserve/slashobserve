import { Link, useRouteContext } from "@tanstack/react-router"

import { AuthorMark } from "@/components/author-mark"
import { DataGrid, type DataGridColumn } from "@/components/data-grid/data-grid"
import { markOf } from "@/lib/author"
import { roleAtLeast } from "@/lib/roles"

import { PresenceStatus } from "./connected-table"
import { agentLink } from "./agent-ref"
import type { ServiceAccountRow } from "./queries"

export function accountMark(row: ServiceAccountRow) {
  return markOf({
    kind: "service_account",
    id: row.id,
    name: row.name,
    origin: null,
    client: null,
    run_id: null,
  })
}

function accountColumns(
  showStats: boolean
): DataGridColumn<ServiceAccountRow>[] {
  const stats: DataGridColumn<ServiceAccountRow>[] = [
    {
      id: "runs",
      header: "Runs in 7 days",
      width: 150,
      cell: (row) =>
        row.last_7_days === null ? null : (
          <span className="text-muted-foreground">{row.last_7_days.runs}</span>
        ),
    },
    {
      id: "failed",
      header: "Failed",
      width: 110,
      cell: (row) =>
        row.last_7_days === null ? null : (
          <span className="text-muted-foreground">
            {row.last_7_days.failed}
          </span>
        ),
    },
  ]
  return [
    {
      id: "name",
      header: "Name",
      minWidth: 180,
      flex: 1.5,
      autoHeight: true,
      cell: (row) => (
        <span className="flex items-center gap-2 py-2">
          <AuthorMark mark={accountMark(row)} size={20} />
          <Link
            {...agentLink({ kind: "service_account", id: row.id })}
            className="font-medium wrap-anywhere no-underline hover:underline"
          >
            {row.name}
          </Link>
        </span>
      ),
    },
    {
      id: "status",
      header: "Status",
      minWidth: 140,
      cell: (row) => (
        <span className="text-muted-foreground">
          <PresenceStatus
            presence={row.presence}
            lastSeenAt={row.last_seen_at}
          />
        </span>
      ),
    },
    ...(showStats ? stats : []),
  ]
}

export function ServiceAccountsTable({
  rows,
}: {
  rows: readonly ServiceAccountRow[]
}) {
  const { principal } = useRouteContext({ from: "/_authed" })
  const showStats = rows.some((row) => row.last_7_days !== null)
  return (
    <div className="flex flex-col gap-2">
      <div className="flex flex-wrap items-baseline justify-between gap-x-4">
        <h2 className="text-sm font-semibold">
          Service accounts that take runs
        </h2>
        {roleAtLeast(principal.role, "admin") ? (
          <Link to="/keys" className="text-[13px] underline underline-offset-4">
            API keys
          </Link>
        ) : null}
      </div>
      {rows.length === 0 ? (
        <p className="rounded-card border p-4 text-[13px] text-muted-foreground">
          No service accounts take runs.
        </p>
      ) : (
        <DataGrid
          label="Service accounts that take runs"
          rows={rows}
          columns={accountColumns(showStats)}
          rowId={(row) => row.id}
        />
      )}
    </div>
  )
}
