import { useQuery } from "@tanstack/react-query"

import { loadProblemText } from "@/service-accounts/queries"
import { RelativeTime, useNow } from "@/components/relative-time"
import { StatusLabel } from "@/components/status-label"
import { Cell, HeaderCell, TableCard } from "@/components/table-card"
import { LoadProblem } from "@/components/load-problem"
import { Skeleton } from "@/components/ui/skeleton"
import { initials } from "@/shell/initials"

import { isDeactivated } from "./person-lifecycle"
import { PersonMenu } from "./person-menu"
import { usersQuery } from "./queries"
import { PersonRole } from "./role-select"
import { isOnline } from "./sign-in-policy"
import { SignOutEverywhereButton } from "./sign-out-everywhere-button"

export function PeopleTable() {
  const query = useQuery(usersQuery())
  const now = useNow()

  if (query.isPending) {
    return (
      <div className="flex flex-col gap-3" aria-busy="true">
        {[0, 1, 2].map((index) => (
          <Skeleton key={index} className="h-10 w-full" />
        ))}
      </div>
    )
  }

  if (query.data === undefined) {
    return (
      <LoadProblem
        title="Could not load people"
        detail={loadProblemText(query.error)}
        retrying={query.isFetching}
        onRetry={() => void query.refetch()}
      />
    )
  }

  if (query.data.length === 0) {
    return (
      <p className="py-16 text-center text-sm text-muted-foreground">
        Nobody has signed in yet.
      </p>
    )
  }

  return (
    <TableCard label="People" minWidth={920} stackOnNarrow>
      <thead>
        <tr>
          <HeaderCell>Name</HeaderCell>
          <HeaderCell>Email</HeaderCell>
          <HeaderCell>Role</HeaderCell>
          <HeaderCell>Dashboards</HeaderCell>
          <HeaderCell>API keys</HeaderCell>
          <HeaderCell>Last active</HeaderCell>
          <HeaderCell>
            <span className="sr-only">Actions</span>
          </HeaderCell>
        </tr>
      </thead>
      <tbody>
        {query.data.map((user) => (
          <tr key={user.id} data-user-id={user.id}>
            <Cell>
              <span className="flex items-center gap-2.5">
                <span
                  aria-hidden="true"
                  className="grid size-[26px] flex-none place-items-center rounded-full border text-[11px] font-semibold"
                >
                  {initials(user.name, user.email).slice(0, 1)}
                </span>
                <span className="font-medium wrap-anywhere">{user.name}</span>
              </span>
            </Cell>
            <Cell className="text-muted-foreground">{user.email}</Cell>
            <Cell>
              <PersonRole person={user} />
            </Cell>
            <Cell data-label="Dashboards">{user.dashboard_count}</Cell>
            <Cell data-label="API keys">{user.key_count}</Cell>
            <Cell>
              {isDeactivated(user) ? (
                <StatusLabel tone="none">
                  Deactivated
                  {user.deactivated_by !== null
                    ? ` by ${user.deactivated_by}`
                    : null}
                </StatusLabel>
              ) : isOnline(user.last_seen_at, now) ? (
                <StatusLabel tone="ok">Online now</StatusLabel>
              ) : (
                <span className="text-muted-foreground">
                  <RelativeTime iso={user.last_seen_at} />
                </span>
              )}
            </Cell>
            <Cell data-cell="action" className="text-right">
              <span className="inline-flex items-center justify-end gap-2">
                {isDeactivated(user) ? null : (
                  <SignOutEverywhereButton user={user} />
                )}
                <PersonMenu user={user} />
              </span>
            </Cell>
          </tr>
        ))}
      </tbody>
    </TableCard>
  )
}
