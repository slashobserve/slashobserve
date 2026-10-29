import { useQueries, useQuery } from "@tanstack/react-query"

import { useNow } from "@/components/relative-time"

import { sortKeys } from "./key-status"
import {
  serviceAccountKeysQuery,
  serviceAccountsQuery,
  type ServiceAccount,
  type ServiceAccountKey,
} from "./queries"

type KeyRow = { key: ServiceAccountKey; account: ServiceAccount }

type AllKeys = {
  status: "pending" | "error" | "success"
  error: unknown
  rows: KeyRow[]
  accounts: ServiceAccount[]
  failedAccounts: ServiceAccount[]
  loadingMore: boolean
  fetching: boolean
  refetch: () => void
}

export function useAllKeys(): AllKeys {
  const now = useNow()
  const accountsQuery = useQuery(serviceAccountsQuery())
  const accounts = accountsQuery.data ?? []
  const keyQueries = useQueries({
    queries: accounts.map((account) => serviceAccountKeysQuery(account.id)),
  })

  const hasFailed = (query: {
    data: unknown
    isError: boolean
    errorUpdateCount: number
  }) =>
    query.data === undefined && (query.isError || query.errorUpdateCount > 0)
  const failed = keyQueries.filter(hasFailed)
  const failedAccounts = accounts.filter((_, index) =>
    hasFailed(keyQueries[index])
  )
  const fetching =
    accountsQuery.isFetching || failed.some((query) => query.isFetching)

  function refetch() {
    void accountsQuery.refetch()
    for (const query of failed) void query.refetch()
  }

  const base = {
    accounts,
    failedAccounts,
    loadingMore: false,
    fetching,
    refetch,
  }
  if (accountsQuery.data === undefined) {
    return {
      ...base,
      status: hasFailed(accountsQuery) ? "error" : "pending",
      error: accountsQuery.error,
      rows: [],
    }
  }
  if (failed.length > 0 && failed.length === accounts.length) {
    return { ...base, status: "error", error: failed[0].error, rows: [] }
  }

  const owner = new Map<string, ServiceAccount>()
  for (const account of accounts) owner.set(account.id, account)
  const keys = keyQueries.flatMap((query) => query.data ?? [])
  const rows: KeyRow[] = []
  for (const key of sortKeys(keys, now)) {
    const account = owner.get(key.service_account_id)
    if (account !== undefined) rows.push({ key, account })
  }

  const loading = keyQueries.some(
    (query) => query.data === undefined && !hasFailed(query)
  )
  if (loading && rows.length === 0) {
    return { ...base, status: "pending", error: null, rows }
  }
  return {
    ...base,
    status: "success",
    loadingMore: loading,
    error: failed[0]?.error ?? null,
    rows,
  }
}
