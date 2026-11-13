import { useId } from "react"
import { Link, useRouteContext } from "@tanstack/react-router"

import { AuthorMark } from "@/components/author-mark"
import { Cell, HeaderCell, TableCard } from "@/components/table-card"
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

export function ServiceAccountsTable({
  rows,
}: {
  rows: readonly ServiceAccountRow[]
}) {
  const headingId = useId()
  const { principal } = useRouteContext({ from: "/_authed" })
  const showStats = rows.some((row) => row.last_7_days !== null)
  return (
    <div className="flex flex-col gap-2">
      <div className="flex flex-wrap items-baseline justify-between gap-x-4">
        <h2 id={headingId} className="text-sm font-semibold">
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
        <TableCard labelledBy={headingId} minWidth={520} stackOnNarrow>
          <thead>
            <tr>
              <HeaderCell>Name</HeaderCell>
              <HeaderCell>Status</HeaderCell>
              {showStats ? (
                <>
                  <HeaderCell>Runs in 7 days</HeaderCell>
                  <HeaderCell>Failed</HeaderCell>
                </>
              ) : null}
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => (
              <tr key={row.id}>
                <Cell>
                  <span className="flex items-center gap-2">
                    <AuthorMark mark={accountMark(row)} size={20} />
                    <Link
                      {...agentLink({ kind: "service_account", id: row.id })}
                      className="font-medium wrap-anywhere no-underline hover:underline"
                    >
                      {row.name}
                    </Link>
                  </span>
                </Cell>
                <Cell className="text-muted-foreground">
                  <PresenceStatus
                    presence={row.presence}
                    lastSeenAt={row.last_seen_at}
                  />
                </Cell>
                {showStats ? (
                  <>
                    <Cell className="text-muted-foreground">
                      {row.last_7_days?.runs ?? ""}
                    </Cell>
                    <Cell className="text-muted-foreground">
                      {row.last_7_days?.failed ?? ""}
                    </Cell>
                  </>
                ) : null}
              </tr>
            ))}
          </tbody>
        </TableCard>
      )}
    </div>
  )
}
