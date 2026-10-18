import { useMemo } from "react"
import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient,
} from "@tanstack/react-query"

import { api } from "@/api/client"
import { ApiError, unwrap } from "@/api/errors"
import { dashboardKeys, observeKeys } from "@/api/keys"
import type { components } from "@/api/schema.gen"
import { latestDashboardQuery } from "@/dashboards/queries"
import { agentOffMessage, sentence, useAgentInfo } from "@/observe/queries"

export type Revision = components["schemas"]["Revision"]
export type RevisionStatusValue = Revision["status"]
export type RevisionProgress = Pick<Revision, "id" | "status"> &
  Partial<Pick<Revision, "attempts" | "result_version" | "error">>

export const maxQueuedRevisions = 5
export const maxPromptLength = 4000

export const tooManyRevisionsText = `This dashboard already has ${maxQueuedRevisions} revisions waiting.`

const pageSize = 50
const safetyPollMs = 5000

export function isActive(status: RevisionStatusValue): boolean {
  return status === "queued" || status === "running"
}

export function revisionsQuery(id: string) {
  return queryOptions({
    queryKey: dashboardKeys.revisions(id),
    queryFn: ({ signal }) =>
      api
        .GET("/dashboards/{id}/revisions", {
          params: { path: { id }, query: { limit: pageSize } },
          signal,
        })
        .then(unwrap)
        .then((body) => body.revisions),
    refetchInterval: (query) =>
      query.state.data?.some((revision) => isActive(revision.status))
        ? safetyPollMs
        : false,
  })
}

export function useRevisions(id: string) {
  return useQuery(revisionsQuery(id))
}

export function useRevisionById(id: string): Map<string, Revision> {
  const { data } = useRevisions(id)
  return useMemo(
    () => new Map((data ?? []).map((revision) => [revision.id, revision])),
    [data]
  )
}

export function useLatestVersion(id: string): number | undefined {
  const queryClient = useQueryClient()
  return useQuery({
    ...latestDashboardQuery(queryClient, id),
    staleTime: Infinity,
    select: (envelope) => envelope.dashboard.latest_version,
  }).data
}

export function isAgentUnavailable(error: unknown): boolean {
  return (
    error instanceof ApiError &&
    error.status === 503 &&
    error.code === "agent_unavailable"
  )
}

export function isTooManyRevisions(error: unknown): boolean {
  return (
    error instanceof ApiError &&
    error.status === 429 &&
    error.code === "too_many_revisions"
  )
}

export function markAgentUnavailable(queryClient: QueryClient) {
  queryClient.setQueryData<components["schemas"]["AgentInfo"]>(
    observeKeys.agent(),
    { available: false, provider: null, model: null }
  )
}

export function useAgentAvailable(): boolean {
  return useAgentInfo().data?.available !== false
}

export function describeRevisionError(error: unknown): string {
  if (isTooManyRevisions(error)) return tooManyRevisionsText
  if (isAgentUnavailable(error)) return agentOffMessage
  if (error instanceof ApiError) {
    return error.status >= 500
      ? "The request could not be sent right now. Try again."
      : sentence(error.message)
  }
  return "The request could not be sent. Check your connection and try again."
}

export function useRequestRevision(id: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: { prompt: string; baseVersion: number }) =>
      api
        .POST("/dashboards/{id}/revisions", {
          params: { path: { id } },
          body: { prompt: input.prompt, base_version: input.baseVersion },
        })
        .then(unwrap)
        .then((body) => body.revision),
    onSuccess: (revision) => {
      queryClient.setQueryData<Revision[]>(
        dashboardKeys.revisions(id),
        (data) =>
          data && !data.some((item) => item.id === revision.id)
            ? [revision, ...data]
            : data
      )
      void queryClient.invalidateQueries({
        queryKey: dashboardKeys.revisions(id),
      })
    },
    onError: (error) => {
      if (isAgentUnavailable(error)) markAgentUnavailable(queryClient)
    },
  })
}
