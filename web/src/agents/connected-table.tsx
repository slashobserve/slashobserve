import { useId } from "react"
import { Link } from "@tanstack/react-router"

import { AuthorMark } from "@/components/author-mark"
import { Cell, HeaderCell, TableCard } from "@/components/table-card"
import { Button } from "@/components/ui/button"
import { ModeSelect } from "@/connected-agents/mode-select"
import { markOf } from "@/lib/author"

import { agentLink } from "./agent-ref"
import type { ConnectedRow } from "./queries"
import { presenceLabel, type Presence } from "./stats"

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

export function ConnectedTable({ rows, onConnect }: Props) {
  const headingId = useId()
  return (
    <div className="flex flex-col gap-2">
      <h2 id={headingId} className="text-sm font-semibold">
        Connected agents
      </h2>
      {rows.length === 0 ? (
        <EmptyConnected onConnect={onConnect} />
      ) : (
        <TableCard labelledBy={headingId} minWidth={640} stackOnNarrow>
          <thead>
            <tr>
              <HeaderCell>Agent</HeaderCell>
              <HeaderCell>Mode</HeaderCell>
              <HeaderCell>Status</HeaderCell>
              <HeaderCell>Writes in 7 days</HeaderCell>
              <HeaderCell>
                <span className="sr-only">Actions</span>
              </HeaderCell>
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => (
              <tr key={row.grant_id}>
                <Cell>
                  <span className="flex items-center gap-2">
                    <AuthorMark mark={grantMark(row)} size={20} />
                    <Link
                      {...agentLink({ kind: "grant", id: row.grant_id })}
                      className="font-medium wrap-anywhere no-underline hover:underline"
                    >
                      {row.client_name}
                    </Link>
                  </span>
                </Cell>
                <Cell>
                  <ModeSelect
                    grantId={row.grant_id}
                    mode={row.mode}
                    name={row.client_name}
                  />
                </Cell>
                <Cell className="text-muted-foreground">
                  <PresenceStatus
                    presence={row.presence}
                    lastSeenAt={row.last_seen_at}
                  />
                </Cell>
                <Cell className="text-muted-foreground">{row.writes_7d}</Cell>
                <Cell data-cell="action" className="text-right">
                  <ManageLink name={row.client_name} />
                </Cell>
              </tr>
            ))}
          </tbody>
        </TableCard>
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
