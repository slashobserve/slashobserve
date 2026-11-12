import {
  queryOptions,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import {
  dashboardKeys,
  proposalKeys,
  threadKeys,
  type ProposalState,
} from "@/api/keys"
import type { components } from "@/api/schema.gen"

export type Proposal = components["schemas"]["Proposal"]
export type ProposalSummary = components["schemas"]["ProposalSummary"]
export type MergeConflict = components["schemas"]["MergeConflict"]
export type MergeChoice = components["schemas"]["MergeChoice"]

const proposalsPage = 50

export function proposalQuery(id: string) {
  return queryOptions({
    queryKey: proposalKeys.detail(id),
    queryFn: ({ signal }) =>
      api
        .GET("/proposals/{id}", { params: { path: { id } }, signal })
        .then(unwrap),
    staleTime: 10_000,
  })
}

export function dashboardProposalsQuery(
  dashboardId: string,
  state: ProposalState = "open"
) {
  return queryOptions({
    queryKey: proposalKeys.list(dashboardId, state),
    queryFn: ({ signal }) =>
      api
        .GET("/dashboards/{id}/proposals", {
          params: {
            path: { id: dashboardId },
            query: { state, limit: proposalsPage },
          },
          signal,
        })
        .then(unwrap),
    staleTime: 10_000,
  })
}

export function useMergeProposal(id: string) {
  const queryClient = useQueryClient()
  return useMutation({
    networkMode: "always",
    mutationFn: (input: { revision: number; choices: MergeChoice[] }) =>
      api
        .POST("/proposals/{id}/merge", {
          params: { path: { id } },
          body: {
            revision: input.revision,
            ...(input.choices.length > 0 ? { choices: input.choices } : {}),
          },
        })
        .then(unwrap),
    onSuccess: (merged) =>
      Promise.all([
        queryClient.invalidateQueries({ queryKey: proposalKeys.all }),
        queryClient.invalidateQueries({
          queryKey: dashboardKeys.one(merged.dashboard_id),
        }),
        queryClient.invalidateQueries({ queryKey: dashboardKeys.list() }),
        queryClient.invalidateQueries({
          queryKey: threadKeys.list(`proposal:${id}`),
        }),
      ]),
  })
}

export function useDiscardProposal(id: string) {
  const queryClient = useQueryClient()
  return useMutation({
    networkMode: "always",
    mutationFn: () =>
      api
        .POST("/proposals/{id}/discard", {
          params: { path: { id } },
          body: {},
        })
        .then(unwrap),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: proposalKeys.all }),
  })
}
