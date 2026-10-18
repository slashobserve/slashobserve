import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query"

import { api } from "@/api/client"
import { ApiError, maxRetryDelayMs, unwrap } from "@/api/errors"
import { observeKeys } from "@/api/keys"
import type { paths } from "@/api/schema.gen"

export type ObserveRequest =
  paths["/observe/{id}"]["get"]["responses"][200]["content"]["application/json"]["request"]

export const agentOffMessage =
  "No LLM provider is configured on this server, so the agent is off."

const agentBusyMessage =
  "The agent is busy with other requests; try again in a moment."

const tooManyRequestsMessage = "Too many requests; try again in a moment."

const pollIntervalMs = 1000

const maxPollBackoffMs = 8000

const stoppingStatuses = new Set([400, 401, 403, 404])

export function sentence(text: string): string {
  const trimmed = text.trimEnd()
  if (trimmed === "") return ""
  const capitalized = trimmed.charAt(0).toUpperCase() + trimmed.slice(1)
  return /[.!?]$/.test(capitalized) ? capitalized : `${capitalized}.`
}

type FailureRun = { dataUpdates: number; errorsBefore: number }

const failureRuns = new WeakMap<object, FailureRun>()

export function pollFailures(query: {
  state: { dataUpdateCount: number; errorUpdateCount: number }
}): number {
  const { dataUpdateCount, errorUpdateCount } = query.state
  let run = failureRuns.get(query)
  if (run === undefined) {
    run = { dataUpdates: 0, errorsBefore: 0 }
    failureRuns.set(query, run)
  }
  if (run.dataUpdates !== dataUpdateCount) {
    run.dataUpdates = dataUpdateCount
    run.errorsBefore = errorUpdateCount
  }
  return errorUpdateCount - run.errorsBefore
}

export function stopsPolling(error: unknown): boolean {
  return error instanceof ApiError && stoppingStatuses.has(error.status)
}

export function pollDelayMs(error: unknown, failures: number): number {
  if (error === null) return pollIntervalMs
  if (error instanceof ApiError && error.retryAfterMs !== undefined) {
    return Math.max(
      pollIntervalMs,
      Math.min(error.retryAfterMs, maxRetryDelayMs)
    )
  }
  return Math.min(
    pollIntervalMs * 2 ** Math.max(failures - 1, 0),
    maxPollBackoffMs
  )
}

export function isTerminal(request: ObserveRequest | undefined): boolean {
  return request?.status === "succeeded" || request?.status === "failed"
}

export function useAgentInfo() {
  return useQuery({
    queryKey: observeKeys.agent(),
    queryFn: ({ signal }) => api.GET("/agent", { signal }).then(unwrap),
    staleTime: 60_000,
  })
}

export function observeRequestQuery(id: string) {
  return queryOptions({
    queryKey: observeKeys.request(id),
    queryFn: ({ signal }) =>
      api
        .GET("/observe/{id}", { params: { path: { id } }, signal })
        .then(unwrap)
        .then((body) => body.request),
    staleTime: 0,
    refetchInterval: (query) => {
      if (isTerminal(query.state.data)) return false
      const { error } = query.state
      if (stopsPolling(error)) return false
      return pollDelayMs(error, pollFailures(query))
    },
  })
}

export function useObserveRequest(id: string) {
  return useQuery(observeRequestQuery(id))
}

export function usePollFailures(id: string): number {
  const query = useQueryClient()
    .getQueryCache()
    .find({ queryKey: observeKeys.request(id), exact: true })
  return query === undefined ? 0 : pollFailures(query)
}

export type ObserveInput = { prompt: string; dashboardId?: string }

export function useCreateObserveRequest() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ prompt, dashboardId }: ObserveInput) =>
      api
        .POST("/observe", { body: { prompt, dashboard_id: dashboardId } })
        .then(unwrap)
        .then((body) => body.request),
    onSuccess: (request) => {
      queryClient.setQueryData(observeKeys.request(request.id), request)
    },
    onError: (error) => {
      if (isAgentUnavailable(error)) {
        void queryClient.invalidateQueries({ queryKey: observeKeys.agent() })
      }
    },
  })
}

function isAgentUnavailable(error: unknown): boolean {
  return (
    error instanceof ApiError &&
    error.status === 503 &&
    error.code === "agent_unavailable"
  )
}

export function describeSubmitError(error: unknown): {
  message: string
  inline: boolean
  invalid: boolean
} {
  if (error instanceof ApiError) {
    if (error.status === 400) {
      return { message: sentence(error.message), inline: true, invalid: true }
    }
    if (error.status === 429) {
      const message =
        error.code === "agent_busy"
          ? agentBusyMessage
          : error.code === "too_many_requests" ||
              error.code === "too_many_revisions"
            ? `${sentence(error.message)} Try again when one finishes.`
            : tooManyRequestsMessage
      return { message, inline: true, invalid: false }
    }
    if (isAgentUnavailable(error)) {
      return { message: agentOffMessage, inline: true, invalid: false }
    }
    return { message: sentence(error.message), inline: false, invalid: false }
  }
  const message =
    error instanceof Error && error.message !== ""
      ? sentence(error.message)
      : "The request could not be sent."
  return { message, inline: false, invalid: false }
}
