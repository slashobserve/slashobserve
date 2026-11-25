import { useQuery } from "@tanstack/react-query"

import { loadProblemText } from "@/service-accounts/queries"
import { RelativeTime, useNow } from "@/components/relative-time"
import { StatusLabel } from "@/components/status-label"
import { DataGrid, type DataGridColumn } from "@/components/data-grid/data-grid"
import { LoadProblem } from "@/components/load-problem"
import { Skeleton } from "@/components/ui/skeleton"
import { initials } from "@/shell/initials"

import { isDeactivated } from "./person-lifecycle"
import { PersonMenu } from "./person-menu"
import { usersQuery, type User } from "./queries"
import { PersonRole } from "./role-select"
import { isOnline } from "./sign-in-policy"
import { SignOutEverywhereButton } from "./sign-out-everywhere-button"

function personId(user: User): string {
  return user.id
}

function peopleColumns(now: number): DataGridColumn<User>[] {
  return [
    {
      id: "name",
      header: "Name",
      minWidth: 160,
      autoHeight: true,
      cell: (user) => (
        <span className="flex items-center gap-2.5 py-2">
          <span
            aria-hidden="true"
            className="grid size-[26px] flex-none place-items-center rounded-full border text-[11px] font-semibold"
          >
            {initials(user.name, user.email).slice(0, 1)}
          </span>
          <span className="font-medium wrap-break-word">{user.name}</span>
        </span>
      ),
    },
    {
      id: "email",
      header: "Email",
      minWidth: 180,
      autoHeight: true,
      cell: (user) => (
        <span className="py-2 wrap-anywhere text-muted-foreground">
          {user.email}
        </span>
      ),
    },
    {
      id: "role",
      header: "Role",
      minWidth: 140,
      cell: (user) => <PersonRole person={user} />,
    },
    {
      id: "dashboards",
      header: "Dashboards",
      width: 110,
      cell: (user) => user.dashboard_count,
    },
    {
      id: "keys",
      header: "API keys",
      width: 100,
      cell: (user) => user.key_count,
    },
    {
      id: "lastActive",
      header: "Last active",
      minWidth: 140,
      cell: (user) =>
        isDeactivated(user) ? (
          <StatusLabel tone="none">
            Deactivated
            {user.deactivated_by !== null ? ` by ${user.deactivated_by}` : null}
          </StatusLabel>
        ) : isOnline(user.last_seen_at, now) ? (
          <StatusLabel tone="ok">Online now</StatusLabel>
        ) : (
          <span className="text-muted-foreground">
            <RelativeTime iso={user.last_seen_at} />
          </span>
        ),
    },
    {
      id: "actions",
      header: "Actions",
      action: true,
      align: "right",
      minWidth: 180,
      cell: (user) => (
        <span className="inline-flex items-center justify-end gap-2">
          {isDeactivated(user) ? null : <SignOutEverywhereButton user={user} />}
          <PersonMenu user={user} />
        </span>
      ),
    },
  ]
}

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
    <DataGrid
      label="People"
      rows={query.data}
      columns={peopleColumns(now)}
      rowId={personId}
      rowAttributes={(user) => ({ "data-user-id": user.id })}
    />
  )
}
