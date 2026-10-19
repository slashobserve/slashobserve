import {
  queryOptions,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query"

import { api } from "@/api/client"
import { ApiError, NetworkError, unwrap } from "@/api/errors"
import { serviceAccountKeys } from "@/api/keys"
import type { paths } from "@/api/schema.gen"
import { sentence } from "@/observe/queries"

export type ServiceAccount =
  paths["/service-accounts"]["post"]["responses"][201]["content"]["application/json"]

export type ServiceAccountKey =
  paths["/service-accounts/{id}/keys"]["get"]["responses"][200]["content"]["application/json"]["items"][number]

export function serviceAccountsQuery() {
  return queryOptions({
    queryKey: serviceAccountKeys.list(),
    queryFn: ({ signal }) =>
      api.GET("/service-accounts", { signal }).then(unwrap),
    select: (data) => data.items,
  })
}

export function serviceAccountKeysQuery(accountId: string) {
  return queryOptions({
    queryKey: serviceAccountKeys.keys(accountId),
    queryFn: ({ signal }) =>
      api
        .GET("/service-accounts/{id}/keys", {
          params: { path: { id: accountId } },
          signal,
        })
        .then(unwrap),
    select: (data) => data.items,
  })
}

export function useCreateServiceAccount() {
  const queryClient = useQueryClient()
  return useMutation({
    networkMode: "always",
    mutationFn: (input: { name: string; description?: string }) =>
      api.POST("/service-accounts", { body: input }).then(unwrap),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: serviceAccountKeys.list() }),
  })
}

export function useCreateKey() {
  const queryClient = useQueryClient()
  return useMutation({
    gcTime: 0,
    networkMode: "always",
    mutationFn: ({
      accountId,
      name,
      expires_at,
    }: {
      accountId: string
      name: string
      expires_at: string | null
    }) =>
      api
        .POST("/service-accounts/{id}/keys", {
          params: { path: { id: accountId } },
          body: { name, expires_at },
        })
        .then(unwrap),
    onSuccess: (_response, { accountId }) =>
      queryClient.invalidateQueries({
        queryKey: serviceAccountKeys.keys(accountId),
      }),
  })
}

export function useRevokeKey(accountId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    networkMode: "always",
    mutationFn: (keyId: string) =>
      api
        .DELETE("/service-accounts/{id}/keys/{keyId}", {
          params: { path: { id: accountId, keyId } },
        })
        .then(unwrap),
    onSettled: () => {
      void queryClient.invalidateQueries({
        queryKey: serviceAccountKeys.keys(accountId),
      })
    },
  })
}

const unreachableText =
  "Could not reach the server. Check your connection and try again."

export function loadProblemText(error: unknown): string {
  if (error instanceof NetworkError) return unreachableText
  if (error instanceof ApiError) return sentence(error.message)
  return "Something went wrong. Try again."
}

export function problemText(
  error: unknown,
  subject: { accountName?: string; what: "account" | "key" | "revoke" }
): string {
  if (error instanceof NetworkError) return unreachableText
  if (error instanceof ApiError) {
    if (error.status === 409 && error.code === "conflict") {
      return `A service account named ${subject.accountName} already exists.`
    }
    if (error.status === 404) {
      return subject.what === "revoke"
        ? "This key no longer exists."
        : "This service account no longer exists."
    }
    if (error.status === 403) {
      return "Your account cannot manage service accounts."
    }
    return sentence(error.message)
  }
  return "Something went wrong. Try again."
}
