import { useMutation, useQuery } from "@tanstack/react-query"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { connectKeys } from "@/api/keys"
import type { paths } from "@/api/schema.gen"

export type ConnectionRequest =
  paths["/oauth/requests/{id}"]["get"]["responses"][200]["content"]["application/json"]

export type Answer = "approve" | "deny"

export function useConnectionRequest(id: string) {
  return useQuery({
    queryKey: connectKeys.request(id),
    queryFn: ({ signal }) =>
      api
        .GET("/oauth/requests/{id}", { params: { path: { id } }, signal })
        .then(unwrap),
    gcTime: 0,
    staleTime: 0,
    refetchOnReconnect: false,
  })
}

export function useAnswerConnection(id: string) {
  return useMutation({
    gcTime: 0,
    networkMode: "always",
    mutationFn: async (answer: Answer) => {
      const params = { params: { path: { id } } }
      const result =
        answer === "approve"
          ? await api.POST("/oauth/requests/{id}/approve", params).then(unwrap)
          : await api.POST("/oauth/requests/{id}/deny", params).then(unwrap)
      return result.redirect_to
    },
  })
}
