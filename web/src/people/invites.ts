import {
  queryOptions,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query"

import { api } from "@/api/client"
import { type ProblemTexts, unwrap } from "@/api/errors"
import { userKeys } from "@/api/keys"
import type { components } from "@/api/schema.gen"
import type { Role } from "@/lib/roles"

export type Invite = components["schemas"]["Invite"]

const pageSize = 500
const maxPages = 20
const maxEmailBytes = 254
const invitesKey = [...userKeys.all, "invites"] as const

async function fetchAllInvites(signal: AbortSignal): Promise<Invite[]> {
  const invites: Invite[] = []
  const seen = new Set<string>()
  let cursor: string | undefined
  for (let pages = 0; pages < maxPages; pages++) {
    const page = await api
      .GET("/invites", {
        params: { query: { limit: pageSize, cursor } },
        signal,
      })
      .then(unwrap)
    invites.push(...page.invites)
    cursor = page.next_cursor ?? undefined
    if (cursor === undefined || seen.has(cursor)) break
    seen.add(cursor)
  }
  return invites
}

export function invitesQuery() {
  return queryOptions({
    queryKey: invitesKey,
    queryFn: ({ signal }) => fetchAllInvites(signal),
    staleTime: 30_000,
    refetchInterval: 60_000,
  })
}

export function inviteEmailProblem(email: string): string | null {
  const trimmed = email.trim()
  if (trimmed === "") return "Enter an email address."
  const at = trimmed.lastIndexOf("@")
  if (/\s/.test(trimmed) || at < 1 || at === trimmed.length - 1) {
    return "Enter one email address, like name@example.com."
  }
  if (new TextEncoder().encode(trimmed).length > maxEmailBytes) {
    return "Email addresses are at most 254 characters."
  }
  return null
}

export const inviteProblems: ProblemTexts = {
  codes: {
    already_member:
      "Someone with this email has already signed in. Change their role in the People table instead.",
    email_domain_not_allowed:
      "People with this email cannot sign in. Add its domain to the allowed email domains in the sign-in settings first.",
  },
  notFound:
    "This invite no longer exists. It was withdrawn, or the person has signed in.",
}

export function useSaveInvite() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ email, role }: { email: string; role: Role }) =>
      api
        .POST("/invites", { body: { email: email.trim(), role } })
        .then(unwrap),
    onSettled: () => queryClient.invalidateQueries({ queryKey: invitesKey }),
  })
}

export function useWithdrawInvite() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      api.DELETE("/invites/{id}", { params: { path: { id } } }).then(unwrap),
    onSettled: () => queryClient.invalidateQueries({ queryKey: invitesKey }),
  })
}
