import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query"

import { api } from "@/api/client"
import { ApiError, unwrap } from "@/api/errors"
import { dashboardKeys, observeKeys } from "@/api/keys"
import type { paths } from "@/api/schema.gen"

export type ObserveRequest =
  paths["/observe/{id}"]["get"]["responses"][200]["content"]["application/json"]["request"]

export const agentOffMessage =
  "No LLM provider is configured on this server, so the agent is off."

const tooManyRequestsMessage =
  "Too many requests are running; try again in a moment."

const pollIntervalMs = 1000

export function sentence(text: string): string {
  return text.charAt(0).toUpperCase() + text.slice(1)
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
      const error = query.state.error
      if (error instanceof ApiError && error.status < 500) return false
      return pollIntervalMs
    },
  })
}

export function useObserveRequest(id: string) {
  return useQuery(observeRequestQuery(id))
}

export function useCreateObserveRequest() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (prompt: string) =>
      api
        .POST("/observe", { body: { prompt } })
        .then(unwrap)
        .then((body) => body.request),
    onSuccess: (request) => {
      queryClient.setQueryData(observeKeys.request(request.id), request)
      void queryClient.invalidateQueries({ queryKey: dashboardKeys.list() })
    },
  })
}

export function describeSubmitError(error: unknown): {
  message: string
  inline: boolean
} {
  if (error instanceof ApiError) {
    if (error.status === 400) {
      return { message: sentence(error.message), inline: true }
    }
    if (error.status === 429) {
      return { message: tooManyRequestsMessage, inline: true }
    }
    if (error.status === 503) return { message: agentOffMessage, inline: true }
    return { message: sentence(error.message), inline: false }
  }
  const message =
    error instanceof Error && error.message !== ""
      ? error.message
      : "The request could not be sent."
  return { message, inline: false }
}
