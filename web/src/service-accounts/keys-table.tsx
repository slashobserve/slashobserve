import type { RefObject } from "react"
import { useQuery } from "@tanstack/react-query"

import { LoadProblem } from "@/components/load-problem"
import { RelativeTime, useNow } from "@/components/relative-time"
import { StatusLabel } from "@/components/status-label"
import { Cell, HeaderCell, TableCard } from "@/components/table-card"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { usersQuery } from "@/people/queries"

import { expiryState, keyStatus } from "./key-status"
import { loadProblemText } from "./queries"
import { RevokeKeyButton } from "./revoke-key-button"
import { useAllKeys } from "./use-all-keys"

type Props = {
  onCreate: () => void
  revokeFocus: RefObject<HTMLElement | null>
}

function problemDetail(error: unknown): string {
  return error === null ? "Trying again." : loadProblemText(error)
}

function Header() {
  return (
    <thead>
      <tr>
        <HeaderCell>Name</HeaderCell>
        <HeaderCell>Key</HeaderCell>
        <HeaderCell>Created by</HeaderCell>
        <HeaderCell>Last used</HeaderCell>
        <HeaderCell>Expires</HeaderCell>
        <HeaderCell>
          <span className="sr-only">Actions</span>
        </HeaderCell>
      </tr>
    </thead>
  )
}

export function KeysTable({ onCreate, revokeFocus }: Props) {
  const now = useNow()
  const keys = useAllKeys()
  const users = useQuery(usersQuery())

  if (keys.status === "pending") {
    return (
      <TableCard label="API keys" minWidth={760}>
        <Header />
        <tbody aria-busy="true">
          {[0, 1, 2].map((index) => (
            <tr key={index}>
              <Cell colSpan={6}>
                <Skeleton className="h-8 w-full" />
              </Cell>
            </tr>
          ))}
        </tbody>
      </TableCard>
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

  function createdBy(id: string | null): string {
    if (users.isError) return "-"
    if (id === null) return "Unknown"
    return names.get(id) ?? "Unknown"
  }

  return (
    <>
      {problem}
      <TableCard label="API keys" minWidth={760}>
        <Header />
        <tbody aria-busy={keys.loadingMore}>
          {keys.rows.map(({ key, account }) => {
            const state = expiryState(key, now)
            return (
              <tr key={key.id} data-key-id={key.id}>
                <Cell>
                  <div className="font-medium wrap-anywhere">{key.name}</div>
                  <div className="text-xs text-muted-foreground">
                    {account.name}
                  </div>
                </Cell>
                <Cell>
                  <span className="font-mono text-xs">
                    {key.display_prefix}…
                  </span>
                </Cell>
                <Cell className="text-muted-foreground">
                  {users.isPending ? (
                    <Skeleton className="h-4 w-16" />
                  ) : (
                    createdBy(key.created_by)
                  )}
                </Cell>
                <Cell className="text-muted-foreground">
                  <RelativeTime iso={key.last_used_at} fallback="Never" />
                </Cell>
                <Cell>
                  <StatusLabel tone={state.tone}>{state.text}</StatusLabel>
                </Cell>
                <Cell className="text-right">
                  {keyStatus(key, now) === "active" ? (
                    <RevokeKeyButton
                      account={account}
                      serviceKey={key}
                      finalFocus={revokeFocus}
                    />
                  ) : null}
                </Cell>
              </tr>
            )
          })}
          {keys.loadingMore ? (
            <tr>
              <Cell colSpan={6}>
                <Skeleton className="h-8 w-full" />
              </Cell>
            </tr>
          ) : null}
        </tbody>
      </TableCard>
    </>
  )
}
