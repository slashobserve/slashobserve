import { useMemo } from "react"
import {
  infiniteQueryOptions,
  type QueryClient,
  type QueryKey,
  queryOptions,
  useInfiniteQuery,
  useIsMutating,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query"

import { api } from "@/api/client"
import {
  ApiError,
  maxRetryDelayMs,
  resendOnceOffline,
  problemText,
  sentence,
  unwrap,
} from "@/api/errors"
import {
  type ChatListFilters,
  dashboardKeys,
  observationKeys,
  observeKeys,
  threadKeys,
} from "@/api/keys"
import type { components, paths } from "@/api/schema.gen"
import type { StatusTone } from "@/components/status-label"
import { askZone } from "@/lib/time"

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

function succeededView(
  r: Pick<RequestSummary, "result_version" | "result_kind">
): { tone: StatusTone; text: string } {
  if (r.result_version !== null) {
    return { tone: "ok", text: `Saved version ${r.result_version}` }
  }
  switch (r.result_kind) {
    case "observation":
    case "answer":
      return { tone: "ok", text: "Answered" }
    case "none":
      return { tone: "warning", text: "Nothing saved" }
    default:
      return { tone: "ok", text: "Saved" }
  }
}

export function requestStatusView(
  r: Pick<
    RequestSummary,
    | "status"
    | "dashboard_title"
    | "error"
    | "result_version"
    | "result_kind"
    | "failure_code"
  >
): { tone: StatusTone; text: string } {
  switch (r.status) {
    case "queued":
    case "running":
      return { tone: "running", text: "Working" }
    case "succeeded":
      return succeededView(r)
    case "failed":
      if (r.result_kind === "partial_version") {
        return {
          tone: "warning",
          text:
            r.result_version === null
              ? "Saved part of the dashboard"
              : `Saved part of the dashboard, version ${r.result_version}`,
        }
      }
      if (r.failure_code === "canceled") {
        return { tone: "warning", text: "You stopped this run" }
      }
      return {
        tone: "critical",
        text: sentence(r.error ?? "The request failed"),
      }
  }
}

function myRequestsQuery(filters: ChatListFilters, limit: number) {
  return infiniteQueryOptions({
    queryKey: observeKeys.mine(filters, limit),
    queryFn: ({ pageParam, signal }) =>
      api
        .GET("/observe", {
          params: {
            query: {
              q: filters.q,
              pinned: filters.pinned,
              dashboard_id: filters.dashboardId,
              observation_id: filters.observationId,
              chat_id: filters.chatId,
              scope: filters.scope,
              state: filters.states,
              anchored: filters.anchored,
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

export function useMyRequests(
  filters: ChatListFilters,
  limit = 20,
  enabled = true,
  pollsWhileWorking = true
) {
  const query = useInfiniteQuery({
    ...myRequestsQuery(filters, limit),
    enabled,
    ...(pollsWhileWorking ? {} : { refetchInterval: false as const }),
  })
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

export function agentInfoQuery() {
  return queryOptions({
    queryKey: observeKeys.agent(),
    queryFn: ({ signal }) => api.GET("/agent", { signal }).then(unwrap),
    staleTime: 60_000,
  })
}

export function useAgentInfo() {
  return useQuery(agentInfoQuery())
}

export function observeRequestQuery(id: string) {
  return queryOptions({
    queryKey: observeKeys.request(id),
    queryFn: ({ signal }) =>
      api
        .GET("/observe/{id}", { params: { path: { id } }, signal })
        .then(unwrap)
        .then((body) => body.request),
    staleTime: 2_000,
    refetchIntervalInBackground: true,
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

type ObserveKind = NonNullable<
  paths["/observe"]["post"]["requestBody"]["content"]["application/json"]["kind"]
>

export type ObserveTurn = {
  kind: ObserveKind
  chatId?: string
  clientId?: string
  dashboardId?: string
  observationId?: string
  panelId?: string
  anchor?: components["schemas"]["Anchor"]
  blockId?: string
  from?: string
  to?: string
  timeZone?: string
}

type AppliedChange = {
  kind: "revise"
  appliesFindingId: string
  dashboardId: string
}

export type ObserveInput = ObserveTurn & ({ prompt: string } | AppliedChange)

function recordRequest(queryClient: QueryClient, request: ObserveRequest) {
  queryClient.setQueryData(observeKeys.request(request.id), request)
  void queryClient.invalidateQueries({ queryKey: observeKeys.mineAll() })
  if (request.dashboard_id) {
    void queryClient.invalidateQueries({
      queryKey: dashboardKeys.revisions(request.dashboard_id),
    })
  }
}

function optionalZone(zone: string | undefined) {
  return zone === undefined ? {} : { time_zone: zone }
}

export function useCreateObserveRequest() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationKey: observeKeys.create(),
    mutationFn: ({
      kind,
      chatId,
      clientId,
      dashboardId,
      observationId,
      panelId,
      anchor,
      blockId,
      from,
      to,
      timeZone,
      ...asked
    }: ObserveInput) =>
      resendOnceOffline(clientId, () =>
        api.POST("/observe", {
          body: {
            ...("prompt" in asked
              ? { prompt: asked.prompt }
              : { applies_finding_id: asked.appliesFindingId }),
            kind,
            ...(dashboardId === undefined ? {} : { dashboard_id: dashboardId }),
            ...(chatId === undefined ? {} : { chat_id: chatId }),
            ...(clientId === undefined ? {} : { client_id: clientId }),
            ...(panelId === undefined ? {} : { panel_id: panelId }),
            ...(anchor === undefined ? {} : { anchor }),
            ...(blockId === undefined ? {} : { block_id: blockId }),
            ...(from === undefined ? {} : { from }),
            ...(to === undefined ? {} : { to }),
            ...(observationId === undefined
              ? {}
              : { observation_id: observationId }),
            ...optionalZone(askZone(timeZone)),
          },
        })
      )
        .then(unwrap)
        .then((body) => body.request),
    onSuccess: (request) => recordRequest(queryClient, request),
    onError: (error) => {
      if (isAgentUnavailable(error)) {
        void queryClient.invalidateQueries({ queryKey: observeKeys.agent() })
      }
    },
  })
}

function useChatMutation<Input, Output>(
  id: string,
  send: (input: Input) => Promise<Output>,
  alsoRefresh: readonly QueryKey[] = []
) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: send,
    onSettled: () =>
      Promise.all(
        [observeKeys.mineAll(), observeKeys.request(id), ...alsoRefresh].map(
          (queryKey) => queryClient.invalidateQueries({ queryKey })
        )
      ),
  })
}

export function useRenameChat(id: string) {
  return useChatMutation(
    id,
    (title: string) => {
      const trimmed = title.trim()
      return api
        .PATCH("/observe/{id}", {
          params: { path: { id } },
          body: { title: trimmed === "" ? null : trimmed },
        })
        .then(unwrap)
    },
    [observationKeys.all]
  )
}

export function usePinChat(id: string) {
  return useChatMutation(id, (pinned: boolean) =>
    api
      .PATCH("/observe/{id}", { params: { path: { id } }, body: { pinned } })
      .then(unwrap)
  )
}

export function useDeleteChatTurns(ids: readonly string[]) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () =>
      Promise.all(
        ids.map((id) =>
          api.DELETE("/observe/{id}", { params: { path: { id } } }).then(unwrap)
        )
      ),
    onSettled: () =>
      Promise.all([
        queryClient.invalidateQueries({ queryKey: observeKeys.mineAll() }),
        ...ids.map((id) =>
          queryClient.invalidateQueries({ queryKey: observeKeys.request(id) })
        ),
      ]),
  })
}

export function useCancelRequest(id: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () =>
      api
        .POST("/observe/{id}/cancel", { params: { path: { id } } })
        .then(unwrap)
        .then((body) => body.request),
    onSuccess: (request) => recordRequest(queryClient, request),
  })
}

export function useSaveAnswer(id: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () =>
      api
        .POST("/observe/{id}/observation", { params: { path: { id } } })
        .then(unwrap),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: observeKeys.request(id) })
      void queryClient.invalidateQueries({ queryKey: observeKeys.mineAll() })
      void queryClient.invalidateQueries({ queryKey: observationKeys.lists() })
    },
  })
}

export function useShareAnswer(id: string, target: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (threadId?: string) =>
      api
        .POST("/observe/{id}/share", {
          params: { path: { id } },
          body: threadId === undefined ? {} : { thread_id: threadId },
        })
        .then(unwrap),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: threadKeys.list(target) })
      void queryClient.invalidateQueries({ queryKey: observeKeys.request(id) })
    },
  })
}

export function useWithdrawShare(id: string, target: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (threadId: string) =>
      api
        .DELETE("/threads/{thread_id}/share", {
          params: { path: { thread_id: threadId } },
        })
        .then(unwrap),
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: threadKeys.list(target) })
      void queryClient.invalidateQueries({ queryKey: observeKeys.request(id) })
    },
  })
}

export function useRetryRequest(id: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () =>
      api
        .POST("/observe/{id}/retry", { params: { path: { id } } })
        .then(unwrap)
        .then((body) => body.request),
    onSuccess: (request) => recordRequest(queryClient, request),
  })
}

export function useContinueRequest(id: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ note }: { note?: string } = {}) => {
      const trimmed = note?.trim() ?? ""
      return api
        .POST("/observe/{id}/continue", {
          params: { path: { id } },
          body: trimmed === "" ? undefined : { note: trimmed },
        })
        .then(unwrap)
        .then((body) => body.request)
    },
    onSuccess: (request) => {
      recordRequest(queryClient, request)
      void queryClient.invalidateQueries({ queryKey: observeKeys.request(id) })
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
    if (error.status === 409 && error.code === "invalid_transition") {
      return {
        message: "This request can no longer be tried again.",
        inline: false,
        invalid: false,
      }
    }
  }
  return {
    message: problemText(error, { fallback: "The request could not be sent." }),
    inline: false,
    invalid: false,
  }
}
