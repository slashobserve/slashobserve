import {
  queryOptions,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { userKeys } from "@/api/keys"
import type { paths } from "@/api/schema.gen"

export type User =
  paths["/users"]["get"]["responses"][200]["content"]["application/json"]["users"][number]

const pageSize = 100
const maxPages = 100

function activityTime(user: User): number {
  const time = Date.parse(user.last_seen_at)
  return Number.isNaN(time) ? Number.NEGATIVE_INFINITY : time
}

function newestFirst(a: User, b: User): number {
  const left = activityTime(a)
  const right = activityTime(b)
  if (left === right) return 0
  return left > right ? -1 : 1
}

async function fetchAllUsers(signal: AbortSignal): Promise<User[]> {
  const users: User[] = []
  const seen = new Set<string>()
  let cursor: string | undefined
  for (let pages = 0; pages < maxPages; pages++) {
    const page = await api
      .GET("/users", { params: { query: { limit: pageSize, cursor } }, signal })
      .then(unwrap)
    users.push(...page.users)
    cursor = page.next_cursor ?? undefined
    if (cursor === undefined || seen.has(cursor)) break
    seen.add(cursor)
  }
  return users.sort(newestFirst)
}

export function usersQuery() {
  return queryOptions({
    queryKey: userKeys.list(),
    queryFn: ({ signal }) => fetchAllUsers(signal),
    staleTime: 30_000,
    refetchInterval: 60_000,
  })
}

export function useSignOutEverywhere(signedInUserId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      api
        .DELETE("/users/{id}/sessions", { params: { path: { id } } })
        .then(unwrap),
    onSettled: (_data, _error, id) =>
      id === signedInUserId
        ? undefined
        : queryClient.invalidateQueries({ queryKey: userKeys.list() }),
  })
}
