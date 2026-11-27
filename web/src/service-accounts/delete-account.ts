import { useMutation, useQueryClient } from "@tanstack/react-query"

import { api } from "@/api/client"
import { type ProblemTexts, unwrap } from "@/api/errors"
import { agentKeys, serviceAccountKeys } from "@/api/keys"

import { type ServiceAccountKey } from "./queries"

export function liveKeyCounts(
  keys: Pick<ServiceAccountKey, "service_account_id" | "state">[]
): Map<string, number> {
  const counts = new Map<string, number>()
  for (const key of keys) {
    if (key.state !== "active" && key.state !== "expiring") continue
    counts.set(
      key.service_account_id,
      (counts.get(key.service_account_id) ?? 0) + 1
    )
  }
  return counts
}

export function deleteAccountText(name: string, liveKeys: number): string {
  const keys =
    liveKeys === 0
      ? "It has no working keys."
      : liveKeys === 1
        ? "Its 1 working key stops at once."
        : `Its ${liveKeys} working keys stop at once.`
  return `${keys} Anything that uses ${name} must get a new key. What it made stays, under its name.`
}

export const deleteAccountProblems: ProblemTexts = {
  codes: {
    provisioned:
      "This account comes from the service-accounts file. Remove it there and restart the server.",
  },
  notFound: "This service account no longer exists.",
}

export function useDeleteServiceAccount() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      api
        .DELETE("/service-accounts/{id}", { params: { path: { id } } })
        .then(unwrap),
    onSettled: () =>
      Promise.all([
        queryClient.invalidateQueries({ queryKey: serviceAccountKeys.all }),
        queryClient.invalidateQueries({ queryKey: agentKeys.list() }),
      ]),
  })
}
