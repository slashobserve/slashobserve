import { useMutation, useQueryClient } from "@tanstack/react-query"

import { api } from "@/api/client"
import { ApiError, unwrap } from "@/api/errors"
import { userKeys } from "@/api/keys"
import { loadProblemText } from "@/service-accounts/queries"

import type { User } from "./queries"

export type PersonAction = "deactivate" | "reactivate"

export function isDeactivated(user: User): boolean {
  return user.deactivated_at !== null
}

export function actionsFor(user: User, signedInId: string): PersonAction[] {
  if (isDeactivated(user)) return ["reactivate"]
  return user.id === signedInId ? [] : ["deactivate"]
}

export function lifecycleProblemText(error: unknown): string {
  if (error instanceof ApiError && error.status === 404) {
    return "This person no longer exists."
  }
  return loadProblemText(error)
}

export function usePersonAction() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, action }: { id: string; action: PersonAction }) =>
      (action === "deactivate"
        ? api.POST("/users/{id}/deactivate", { params: { path: { id } } })
        : api.POST("/users/{id}/reactivate", { params: { path: { id } } })
      ).then(unwrap),
    onSettled: () =>
      queryClient.invalidateQueries({ queryKey: userKeys.list() }),
  })
}
