import {
  queryOptions,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query"

import { api } from "@/api/client"
import { type ProblemTexts, unwrap } from "@/api/errors"
import { userKeys } from "@/api/keys"
import type { paths } from "@/api/schema.gen"
import type { Role } from "@/lib/roles"

export type User =
  paths["/users"]["get"]["responses"][200]["content"]["application/json"]["users"][number]

const pageSize = 100
const maxPages = 100

export function byNameThenId(a: User, b: User): number {
  return (
    a.name.localeCompare(b.name, undefined, { sensitivity: "base" }) ||
    a.email.localeCompare(b.email, undefined, { sensitivity: "base" }) ||
    a.id.localeCompare(b.id)
  )
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
  return users.sort(byNameThenId)
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

export const setRoleProblems: ProblemTexts = {
  codes: { last_admin: "At least one Admin must remain." },
  notFound: "This person no longer exists.",
}
