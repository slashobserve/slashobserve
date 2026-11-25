import type { RefObject } from "react"
import { useQuery } from "@tanstack/react-query"

import { LoadProblem } from "@/components/load-problem"
import { RelativeTime, useNow } from "@/components/relative-time"
import { StatusLabel } from "@/components/status-label"
import { DataGrid, type DataGridColumn } from "@/components/data-grid/data-grid"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { roleLabels } from "@/lib/roles"
import { usersQuery } from "@/people/queries"

import { canRevokeKey, expiryState } from "./key-status"
import {
  loadProblemText,
  type ServiceAccount,
  type ServiceAccountKey,
} from "./queries"
import { RevokeKeyButton } from "./revoke-key-button"
import { useAllKeys } from "./use-all-keys"

type Props = {
  onCreate: () => void
  revokeFocus: RefObject<HTMLElement | null>
}

function problemDetail(error: unknown): string {
  return error === null ? "Trying again." : loadProblemText(error)
}

type KeyRow = { key: ServiceAccountKey; account: ServiceAccount }

type KeyColumnContext = {
  now: number
  usersPending: boolean
  createdBy: (id: string | null, provisioned: boolean) => string
  revokeFocus: RefObject<HTMLElement | null>
}

function keyColumns({
  now,
  usersPending,
  createdBy,
  revokeFocus,
}: KeyColumnContext): DataGridColumn<KeyRow>[] {
  return [
    {
      id: "name",
      header: "Name",
      minWidth: 160,
      autoHeight: true,
      cell: ({ key, account }) => (
        <div className="py-2">
          <div className="font-medium wrap-anywhere">{key.name}</div>
          <div className="text-xs text-muted-foreground">
            {account.name}
            {account.provisioned ? " - from file" : null}
          </div>
        </div>
      ),
    },
    {
      id: "key",
      header: "Key",
      minWidth: 140,
      cell: ({ key }) => (
        <span className="font-mono text-xs">{key.display_prefix}…</span>
      ),
    },
    {
      id: "role",
      header: "Role",
      width: 110,
      cell: ({ account }) => (
        <span className="text-muted-foreground">
          {roleLabels[account.role]}
        </span>
      ),
    },
    {
      id: "createdBy",
      header: "Created by",
      minWidth: 130,
      cell: ({ key, account }) =>
        usersPending && !account.provisioned ? (
          <Skeleton className="h-4 w-16" />
        ) : (
          <span className="truncate text-muted-foreground">
            {createdBy(key.created_by, account.provisioned)}
          </span>
        ),
    },
    {
      id: "lastUsed",
      header: "Last used",
      width: 120,
      cell: ({ key }) => (
        <span className="text-muted-foreground">
          <RelativeTime iso={key.last_used_at} fallback="Never" />
        </span>
      ),
    },
    {
      id: "expires",
      header: "Expires",
      width: 140,
      cell: ({ key }) => {
        const state = expiryState(key, now)
        return <StatusLabel tone={state.tone}>{state.text}</StatusLabel>
      },
    },
    {
      id: "actions",
      header: "Actions",
      action: true,
      align: "right",
      width: 110,
      cell: ({ key, account }) =>
        canRevokeKey(key, account) ? (
          <RevokeKeyButton
            account={account}
            serviceKey={key}
            finalFocus={revokeFocus}
          />
        ) : null,
    },
  ]
}

export function KeysTable({ onCreate, revokeFocus }: Props) {
  const now = useNow()
  const keys = useAllKeys()
  const users = useQuery(usersQuery())

  if (keys.status === "pending") {
    return (
      <div className="flex flex-col gap-3" aria-busy="true">
        {[0, 1, 2].map((index) => (
          <Skeleton key={index} className="h-10 w-full" />
        ))}
      </div>
    )
  }

  if (keys.status === "error") {
    return (
      <LoadProblem
        title="Could not load API keys"
        detail={problemDetail(keys.error)}
        retrying={keys.fetching}
        onRetry={keys.refetch}
      />
    )
  }

  const problem =
    keys.failedAccounts.length > 0 ? (
      <div className="mb-3">
        <LoadProblem
          title="Could not load some API keys"
          detail={`Keys for ${keys.failedAccounts.map((account) => account.name).join(", ")} did not load. ${problemDetail(keys.error)}`}
          retrying={keys.fetching}
          onRetry={keys.refetch}
        />
      </div>
    ) : null

  if (keys.rows.length === 0 && problem !== null) return problem

  if (keys.rows.length === 0) {
    return (
      <div className="flex flex-col items-center gap-3 rounded-card border px-4 py-10 text-center">
        <div className="flex flex-col gap-1">
          <h2 className="text-sm font-semibold">No keys yet</h2>
          <p className="text-[13px] text-muted-foreground">
            Create a key to connect Claude Code, OpenCode or the observe CLI.
          </p>
        </div>
        <Button onClick={onCreate}>Create key</Button>
      </div>
    )
  }

  const names = new Map(users.data?.map((user) => [user.id, user.name]))

  function createdBy(id: string | null, provisioned: boolean): string {
    if (provisioned) return "Service accounts file"
    if (users.isError) return "-"
    if (id === null) return "Unknown"
    return names.get(id) ?? "Unknown"
  }

  return (
    <>
      {problem}
      <DataGrid
        label="API keys"
        rows={keys.rows}
        columns={keyColumns({
          now,
          usersPending: users.isPending,
          createdBy,
          revokeFocus,
        })}
        rowId={({ key }) => key.id}
        rowAttributes={({ key }) => ({ "data-key-id": key.id })}
      />
      {keys.loadingMore ? (
        <Skeleton className="mt-2 h-8 w-full" aria-busy="true" />
      ) : null}
    </>
  )
}
