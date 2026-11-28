import {
  queryOptions,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query"

import { api } from "@/api/client"
import { type ProblemTexts, unwrap } from "@/api/errors"
import { userKeys } from "@/api/keys"
import type { components } from "@/api/schema.gen"

import type { User } from "./queries"

export type Team = components["schemas"]["Team"]

const maxTeamNameLength = 100
const teamsKey = [...userKeys.all, "teams"] as const

export function teamsQuery() {
  return queryOptions({
    queryKey: teamsKey,
    queryFn: ({ signal }) => api.GET("/teams", { signal }).then(unwrap),
    select: (data) => data.teams,
    staleTime: 30_000,
  })
}

export function teamNameProblem(name: string): string | null {
  const trimmed = name.trim()
  if (trimmed === "") return "Enter a name."
  if ([...trimmed].length > maxTeamNameLength) {
    return "Team names are at most 100 characters."
  }
  return null
}

export const teamProblems: ProblemTexts = {
  codes: {
    team_exists: "A team with this name already exists.",
    too_many_teams: "200 teams already exist. Delete one first.",
  },
  notFound: "This team or person no longer exists.",
}

export function candidatesFor(team: Team, people: readonly User[]): User[] {
  const members = new Set(team.members.map((member) => member.id))
  return people.filter(
    (person) => person.deactivated_at === null && !members.has(person.id)
  )
}

function useTeamMutation<T, R>(run: (input: T) => Promise<R>) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: run,
    onSettled: () => queryClient.invalidateQueries({ queryKey: teamsKey }),
  })
}

export function useCreateTeam() {
  return useTeamMutation((name: string) =>
    api.POST("/teams", { body: { name: name.trim() } }).then(unwrap)
  )
}

export function useDeleteTeam() {
  return useTeamMutation((id: string) =>
    api.DELETE("/teams/{id}", { params: { path: { id } } }).then(unwrap)
  )
}

export function useTeamMember() {
  return useTeamMutation(
    ({ team, user, add }: { team: string; user: string; add: boolean }) => {
      const params = { params: { path: { id: team, user_id: user } } }
      return (
        add
          ? api.PUT("/teams/{id}/members/{user_id}", params)
          : api.DELETE("/teams/{id}/members/{user_id}", params)
      ).then(unwrap)
    }
  )
}
