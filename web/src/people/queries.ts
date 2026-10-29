import {
  queryOptions,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query"

import { api } from "@/api/client"
import { ApiError, unwrap } from "@/api/errors"
import { userKeys } from "@/api/keys"
import type { paths } from "@/api/schema.gen"
import type { Role } from "@/lib/roles"
import { loadProblemText } from "@/service-accounts/queries"

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

export function useSetRole() {
  const queryClient = useQueryClient()
  return useMutation({
    networkMode: "always",
    mutationFn: ({ id, role }: { id: string; role: Role }) =>
      api
        .PATCH("/users/{id}", {
          params: { path: { id } },
          body: { role },
        })
        .then(unwrap),
    onSettled: () =>
      queryClient.invalidateQueries({ queryKey: userKeys.list() }),
  })
}

export function setRoleProblemText(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.code === "last_admin") return "At least one Admin must remain."
    if (error.status === 404) return "This person no longer exists."
  }
  return loadProblemText(error)
}
