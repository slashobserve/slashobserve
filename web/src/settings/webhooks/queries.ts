import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { webhookKeys } from "@/api/keys"
import type { components } from "@/api/schema.gen"

export type Webhook = components["schemas"]["Webhook"]
export type WebhookDelivery = components["schemas"]["WebhookDelivery"]
export type CreateWebhookBody = components["schemas"]["CreateWebhookRequest"]
export type UpdateWebhookBody = components["schemas"]["UpdateWebhookRequest"]

const deliveriesLimit = 20

export function webhooksQuery() {
  return queryOptions({
    queryKey: webhookKeys.list(),
    queryFn: ({ signal }) =>
      api
        .GET("/webhooks", { signal })
        .then(unwrap)
        .then((body) => body.webhooks),
  })
}

function deliveriesQuery(id: string) {
  return queryOptions({
    queryKey: webhookKeys.deliveries(id),
    queryFn: ({ signal }) =>
      api
        .GET("/webhooks/{id}/deliveries", {
          params: { path: { id }, query: { limit: deliveriesLimit } },
          signal,
        })
        .then(unwrap)
        .then((body) => body.deliveries),
  })
}

export function useDeliveries(id: string) {
  return useQuery(deliveriesQuery(id))
}

export function useCreateWebhook() {
  const queryClient = useQueryClient()
  return useMutation({
    gcTime: 0,
    networkMode: "always",
    mutationFn: (body: CreateWebhookBody) =>
      api.POST("/webhooks", { body }).then(unwrap),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: webhookKeys.list() }),
  })
}

export function useUpdateWebhook() {
  const queryClient = useQueryClient()
  return useMutation({
    networkMode: "always",
    mutationFn: ({ id, body }: { id: string; body: UpdateWebhookBody }) =>
      api
        .PATCH("/webhooks/{id}", { params: { path: { id } }, body })
        .then(unwrap),
    onSettled: () =>
      queryClient.invalidateQueries({ queryKey: webhookKeys.list() }),
  })
}

export function useRotateSecret() {
  const queryClient = useQueryClient()
  return useMutation({
    gcTime: 0,
    networkMode: "always",
    mutationFn: (id: string) =>
      api
        .POST("/webhooks/{id}/secret", { params: { path: { id } } })
        .then(unwrap),
    onSettled: () =>
      queryClient.invalidateQueries({ queryKey: webhookKeys.list() }),
  })
}

export function useTestWebhook() {
  const queryClient = useQueryClient()
  return useMutation({
    networkMode: "always",
    mutationFn: (id: string) =>
      api
        .POST("/webhooks/{id}/test", { params: { path: { id } } })
        .then(unwrap),
    onSettled: (_result, _error, id) =>
      queryClient.invalidateQueries({ queryKey: webhookKeys.deliveries(id) }),
  })
}

export function useDeleteWebhook() {
  const queryClient = useQueryClient()
  return useMutation({
    networkMode: "always",
    mutationFn: (id: string) =>
      api.DELETE("/webhooks/{id}", { params: { path: { id } } }).then(unwrap),
    onSettled: () =>
      queryClient.invalidateQueries({ queryKey: webhookKeys.all }),
  })
}
