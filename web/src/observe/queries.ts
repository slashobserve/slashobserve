import { useMemo } from "react"
import {
  infiniteQueryOptions,
  queryOptions,
  useInfiniteQuery,
  useIsMutating,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query"

import { api } from "@/api/client"
import { ApiError, maxRetryDelayMs, unwrap } from "@/api/errors"
import { dashboardKeys, observeKeys } from "@/api/keys"
import type { components, paths } from "@/api/schema.gen"
import type { StatusTone } from "@/components/status-label"

export type ObserveRequest =
  paths["/observe/{id}"]["get"]["responses"][200]["content"]["application/json"]["request"]

export type RequestSummary = components["schemas"]["RequestSummary"]

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

export function requestStatusView(
  r: Pick<
    RequestSummary,
    "status" | "dashboard_title" | "error" | "result_version"
  >
): { tone: StatusTone; text: string } {
  switch (r.status) {
    case "queued":
    case "running":
      return { tone: "running", text: "Working" }
    case "succeeded":
      return { tone: "ok", text: r.dashboard_title ?? "Dashboard saved" }
    case "failed":
      return {
        tone: "critical",
        text: sentence(r.error ?? "The request failed"),
      }
  }
}

function myRequestsQuery(source: "all" | "observe", limit: number) {
  return infiniteQueryOptions({
    queryKey: observeKeys.mine(source, limit),
    queryFn: ({ pageParam, signal }) =>
      api
        .GET("/observe", {
          params: {
            query: {
              source: source === "observe" ? "observe" : undefined,
              limit,
              cursor: pageParam,
            },
          },
          signal,
        })
        .then(unwrap),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor ?? undefined,
    staleTime: 10_000,
    refetchInterval: (query) =>
      query.state.data?.pages.some((page) =>
        page.requests.some(
          (request) =>
            request.status === "queued" || request.status === "running"
        )
      )
        ? 5000
        : false,
  })
}

export function useMyRequests(source: "all" | "observe", limit = 20) {
  const query = useInfiniteQuery(myRequestsQuery(source, limit))
  const requests = useMemo(() => {
    const seen = new Set<string>()
    const unique: RequestSummary[] = []
    for (const page of query.data?.pages ?? []) {
      for (const request of page.requests) {
        if (seen.has(request.id)) continue
        seen.add(request.id)
        unique.push(request)
      }
    }
    return unique
  }, [query.data])
  return { requests, query }
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
    mutationKey: observeKeys.create(),
    networkMode: "always",
    mutationFn: ({ prompt, dashboardId }: ObserveInput) =>
      api
        .POST("/observe", { body: { prompt, dashboard_id: dashboardId } })
        .then(unwrap)
        .then((body) => body.request),
    onSuccess: (request) => {
      queryClient.setQueryData(observeKeys.request(request.id), request)
      void queryClient.invalidateQueries({ queryKey: observeKeys.mineAll() })
      if (request.dashboard_id) {
        void queryClient.invalidateQueries({
          queryKey: dashboardKeys.revisions(request.dashboard_id),
        })
      }
    },
    onError: (error) => {
      if (isAgentUnavailable(error)) {
        void queryClient.invalidateQueries({ queryKey: observeKeys.agent() })
      }
    },
  })
}

export function useCreatePending(): boolean {
  return useIsMutating({ mutationKey: observeKeys.create() }) > 0
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
