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

export function lastActiveAt(user: User): string | null {
  return user.last_seen_at ?? user.last_login_at ?? null
}

function activityTime(user: User): number {
  const iso = lastActiveAt(user)
  const time = iso === null ? Number.NaN : Date.parse(iso)
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
  let cursor: string | undefined
  do {
    const page = await api
      .GET("/users", { params: { query: { limit: pageSize, cursor } }, signal })
      .then(unwrap)
    users.push(...page.users)
    cursor = page.next_cursor ?? undefined
  } while (cursor !== undefined)
  return users.sort(newestFirst)
}

export function usersQuery() {
  return queryOptions({
    queryKey: userKeys.list(),
    queryFn: ({ signal }) => fetchAllUsers(signal),
    staleTime: 30_000,
  })
}

export function useSignOutEverywhere() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      api
        .DELETE("/users/{id}/sessions", { params: { path: { id } } })
        .then(unwrap),
    onSettled: () =>
      queryClient.invalidateQueries({ queryKey: userKeys.list() }),
  })
}
