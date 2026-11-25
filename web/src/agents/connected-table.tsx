import { Link } from "@tanstack/react-router"

import { AuthorMark } from "@/components/author-mark"
import { DataGrid, type DataGridColumn } from "@/components/data-grid/data-grid"
import { Button } from "@/components/ui/button"
import { ModeSelect } from "@/connected-agents/mode-select"
import { markOf } from "@/lib/author"

import { agentLink } from "./agent-ref"
import type { ConnectedRow } from "./queries"
import { presenceLabel, type Presence } from "./stats"

function grantRowId(row: ConnectedRow): string {
  return row.grant_id
}

type Props = {
  rows: readonly ConnectedRow[]
  onConnect: () => void
}

export function PresenceStatus({
  presence,
  lastSeenAt,
}: {
  presence: Presence
  lastSeenAt: string | null
}) {
  return (
    <span className="inline-flex items-center gap-1.5">
      {presence !== "offline" ? (
        <span
          aria-hidden="true"
          className="size-1.5 flex-none rounded-full bg-status-ok"
        />
      ) : null}
      {presenceLabel(presence, lastSeenAt)}
    </span>
  )
}

export function grantMark(row: ConnectedRow) {
  return markOf({
    kind: "user",
    id: row.grant_id,
    name: row.client_name,
    origin: "mcp",
    client: row.client_name,
    run_id: null,
  })
}

const connectedColumns: DataGridColumn<ConnectedRow>[] = [
  {
    id: "agent",
    header: "Agent",
    minWidth: 180,
    flex: 1.5,
    autoHeight: true,
    cell: (row) => (
      <span className="flex items-center gap-2 py-2">
        <AuthorMark mark={grantMark(row)} size={20} />
        <Link
          {...agentLink({ kind: "grant", id: row.grant_id })}
          className="font-medium wrap-anywhere no-underline hover:underline"
        >
          {row.client_name}
        </Link>
      </span>
    ),
  },
  {
    id: "mode",
    header: "Mode",
    minWidth: 150,
    cell: (row) => (
      <ModeSelect
        grantId={row.grant_id}
        mode={row.mode}
        name={row.client_name}
      />
    ),
  },
  {
    id: "status",
    header: "Status",
    minWidth: 140,
    cell: (row) => (
      <span className="text-muted-foreground">
        <PresenceStatus presence={row.presence} lastSeenAt={row.last_seen_at} />
      </span>
    ),
  },
  {
    id: "writes",
    header: "Writes in 7 days",
    width: 150,
    cell: (row) => (
      <span className="text-muted-foreground">{row.writes_7d}</span>
    ),
  },
  {
    id: "actions",
    header: "Actions",
    action: true,
    align: "right",
    width: 160,
    cell: (row) => <ManageLink name={row.client_name} />,
  },
]

export function ConnectedTable({ rows, onConnect }: Props) {
  return (
    <div className="flex flex-col gap-2">
      <h2 className="text-sm font-semibold">Connected agents</h2>
      {rows.length === 0 ? (
        <EmptyConnected onConnect={onConnect} />
      ) : (
        <DataGrid
          label="Connected agents"
          rows={rows}
          columns={connectedColumns}
          rowId={grantRowId}
        />
      )}
    </div>
  )
}

function ManageLink({ name }: { name: string }) {
  return (
    <Link
      to="/settings"
      hash="connected-agents"
      aria-label={`Manage ${name} in Settings`}
      className="text-[13px] underline underline-offset-4"
    >
      Manage in Settings
    </Link>
  )
}

function EmptyConnected({ onConnect }: Pick<Props, "onConnect">) {
  return (
    <div className="flex flex-col items-start gap-2 rounded-card border p-4">
      <p className="text-[13px] text-muted-foreground">
        No connected agents yet.
      </p>
      <Button variant="outline" size="sm" onClick={onConnect}>
        Connect an agent
      </Button>
    </div>
  )
}
