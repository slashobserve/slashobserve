import { useMutation, useQueryClient } from "@tanstack/react-query"

import { api } from "@/api/client"
import { type ProblemTexts, unwrap } from "@/api/errors"
import { userKeys } from "@/api/keys"

import type { User } from "./queries"

type PersonalItems = NonNullable<User["personal_items"]>

export type PersonAction = "deactivate" | "reactivate" | "remove"

export function isDeactivated(user: User): boolean {
  return user.deactivated_at !== null
}

export function actionsFor(user: User, signedInId: string): PersonAction[] {
  if (isDeactivated(user)) return ["reactivate", "remove"]
  return user.id === signedInId ? [] : ["deactivate"]
}

export const lifecycleProblems: ProblemTexts = {
  codes: {
    not_deactivated:
      "Someone reactivated this person. Deactivate them before removing them.",
  },
  notFound: "This person no longer exists.",
}

function runAction(id: string, action: PersonAction): Promise<unknown> {
  const params = { params: { path: { id } } }
  switch (action) {
    case "deactivate":
      return api.POST("/users/{id}/deactivate", params).then(unwrap)
    case "reactivate":
      return api.POST("/users/{id}/reactivate", params).then(unwrap)
    case "remove":
      return api.DELETE("/users/{id}", params).then(unwrap)
  }
}

export function removalText(items: PersonalItems): string {
  const parts = [
    count(items.dashboards, "dashboard"),
    count(items.observations, "observation"),
    count(items.saved_searches, "saved search", "saved searches"),
  ].filter((part) => part !== null)
  if (parts.length === 0) return "Their My Dashboards is empty."
  const list =
    parts.length === 1
      ? parts[0]
      : `${parts.slice(0, -1).join(", ")} and ${parts.at(-1)}`
  return `Their My Dashboards holds ${list}, deleted with them.`
}

function count(n: number, one: string, many = `${one}s`): string | null {
  if (n === 0) return null
  return n === 1 ? `1 ${one}` : `${n} ${many}`
}

export function usePersonAction() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, action }: { id: string; action: PersonAction }) =>
      runAction(id, action),
    onSettled: () =>
      queryClient.invalidateQueries({ queryKey: userKeys.list() }),
  })
}
