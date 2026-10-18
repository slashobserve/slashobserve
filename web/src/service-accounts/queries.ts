import {
  queryOptions,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query"

import { api } from "@/api/client"
import { ApiError, NetworkError, unwrap } from "@/api/errors"
import { instanceKeys, serviceAccountKeys } from "@/api/keys"
import type { paths } from "@/api/schema.gen"
import { sentence } from "@/observe/queries"

export type ServiceAccount =
  paths["/service-accounts"]["post"]["responses"][201]["content"]["application/json"]

export type ServiceAccountKey =
  paths["/service-accounts/{id}/keys"]["get"]["responses"][200]["content"]["application/json"]["items"][number]

export type Instance =
  paths["/instance"]["get"]["responses"][200]["content"]["application/json"]

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

export function instanceQuery() {
  return queryOptions({
    queryKey: instanceKeys.info(),
    queryFn: ({ signal }) => api.GET("/instance", { signal }).then(unwrap),
    staleTime: Infinity,
  })
}

export function useCreateServiceAccount() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: { name: string; description?: string }) =>
      api.POST("/service-accounts", { body: input }).then(unwrap),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: serviceAccountKeys.list() }),
  })
}

export function useCreateKey(accountId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    gcTime: 0,
    mutationFn: (input: { name: string; expires_at: string | null }) =>
      api
        .POST("/service-accounts/{id}/keys", {
          params: { path: { id: accountId } },
          body: input,
        })
        .then(unwrap),
    onSuccess: () =>
      queryClient.invalidateQueries({
        queryKey: serviceAccountKeys.keys(accountId),
      }),
  })
}

export function useRevokeKey(accountId: string) {
  const queryClient = useQueryClient()
  return useMutation({
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

export function problemText(
  error: unknown,
  subject: { accountName?: string; what: "account" | "key" | "revoke" }
): string {
  if (error instanceof NetworkError) {
    return "Could not reach the server. Check your connection and try again."
  }
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
