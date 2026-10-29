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
import { agentOffMessage, sentence } from "@/observe/queries"

export type Revision = components["schemas"]["Revision"]
export type RevisionStatusValue = Revision["status"]

export const maxQueuedRevisions = 5
export const maxPromptLength = 4000

const waitingText = `This dashboard already has ${maxQueuedRevisions} revisions waiting.`
const tooManyRevisionsText = `${waitingText} Try again when one finishes.`

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

export function isAgentUnavailable(error: unknown): boolean {
  return (
    error instanceof ApiError &&
    error.status === 503 &&
    error.code === "agent_unavailable"
  )
}

function isTooManyRevisions(error: unknown): boolean {
  return (
    error instanceof ApiError &&
    error.status === 429 &&
    error.code === "too_many_revisions"
  )
}

function markAgentUnavailable(queryClient: QueryClient) {
  queryClient.setQueryData<components["schemas"]["AgentInfo"]>(
    observeKeys.agent(),
    { available: false, provider: null, model: null, effort: null }
  )
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
    mutationKey: dashboardKeys.posts(id),
    networkMode: "always",
    mutationFn: (input: {
      prompt: string
      baseVersion: number
      panelId?: string | null
      commentId?: string
    }) =>
      api
        .POST("/dashboards/{id}/revisions", {
          params: { path: { id } },
          body: {
            prompt: input.prompt,
            base_version: input.baseVersion,
            ...(input.commentId === undefined
              ? { panel_id: input.panelId ?? undefined }
              : { comment_id: input.commentId }),
          },
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
      void queryClient.invalidateQueries({ queryKey: observeKeys.mineAll() })
    },
    onError: (error) => {
      if (isAgentUnavailable(error)) markAgentUnavailable(queryClient)
    },
  })
}
