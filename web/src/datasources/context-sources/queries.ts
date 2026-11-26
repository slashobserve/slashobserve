import {
  queryOptions,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { contextSourceKeys } from "@/api/keys"
import type { components } from "@/api/schema.gen"

export type ContextSource = components["schemas"]["ContextSource"]
export type ContextTool = components["schemas"]["ContextSourceTool"]
export type CreateContextSourceBody =
  components["schemas"]["CreateContextSourceRequest"]
export type UpdateContextSourceBody =
  components["schemas"]["UpdateContextSourceRequest"]

export function contextSourcesQuery() {
  return queryOptions({
    queryKey: contextSourceKeys.list(),
    queryFn: ({ signal }) =>
      api
        .GET("/context-sources", { signal })
        .then(unwrap)
        .then((body) => body.context_sources),
  })
}

export function contextToolsQuery(uid: string) {
  return queryOptions({
    queryKey: contextSourceKeys.tools(uid),
    queryFn: ({ signal }) =>
      api
        .GET("/context-sources/{uid}/tools", {
          params: { path: { uid } },
          signal,
        })
        .then(unwrap)
        .then((body) => body.tools),
  })
}

export function useCreateContextSource() {
  const queryClient = useQueryClient()
  return useMutation({
    gcTime: 0,
    networkMode: "always",
    mutationFn: (body: CreateContextSourceBody) =>
      api.POST("/context-sources", { body }).then(unwrap),
    onSettled: () =>
      queryClient.invalidateQueries({ queryKey: contextSourceKeys.all }),
  })
}

export function useUpdateContextSource() {
  const queryClient = useQueryClient()
  return useMutation({
    gcTime: 0,
    networkMode: "always",
    mutationFn: ({
      uid,
      body,
    }: {
      uid: string
      body: UpdateContextSourceBody
    }) =>
      api
        .PATCH("/context-sources/{uid}", { params: { path: { uid } }, body })
        .then(unwrap),
    onSettled: () =>
      queryClient.invalidateQueries({ queryKey: contextSourceKeys.all }),
  })
}

export function useDeleteContextSource() {
  const queryClient = useQueryClient()
  return useMutation({
    networkMode: "always",
    mutationFn: (uid: string) =>
      api
        .DELETE("/context-sources/{uid}", { params: { path: { uid } } })
        .then(unwrap),
    onSettled: () =>
      queryClient.invalidateQueries({ queryKey: contextSourceKeys.all }),
  })
}

export function useDiscoverContextSource() {
  const queryClient = useQueryClient()
  return useMutation({
    networkMode: "always",
    mutationFn: (uid: string) =>
      api
        .POST("/context-sources/{uid}/discover", { params: { path: { uid } } })
        .then(unwrap),
    onSettled: () =>
      queryClient.invalidateQueries({ queryKey: contextSourceKeys.all }),
  })
}

export function useApproveContextTool() {
  const queryClient = useQueryClient()
  return useMutation({
    networkMode: "always",
    mutationFn: ({
      uid,
      tool,
    }: {
      uid: string
      tool: Pick<ContextTool, "name" | "hash" | "seen">
    }) =>
      api
        .POST("/context-sources/{uid}/tools/{name}/approve", {
          params: { path: { uid, name: tool.name } },
          body: { hash: tool.seen?.hash ?? tool.hash },
        })
        .then(unwrap),
    onSettled: () =>
      queryClient.invalidateQueries({ queryKey: contextSourceKeys.all }),
  })
}

export function useApproveReadOnlyTools() {
  const queryClient = useQueryClient()
  return useMutation({
    networkMode: "always",
    mutationFn: (uid: string) =>
      api
        .POST("/context-sources/{uid}/tools/approve-read-only", {
          params: { path: { uid } },
        })
        .then(unwrap),
    onSettled: () =>
      queryClient.invalidateQueries({ queryKey: contextSourceKeys.all }),
  })
}

export function useRevokeContextTool() {
  const queryClient = useQueryClient()
  return useMutation({
    networkMode: "always",
    mutationFn: ({ uid, name }: { uid: string; name: string }) =>
      api
        .POST("/context-sources/{uid}/tools/{name}/revoke", {
          params: { path: { uid, name } },
        })
        .then(unwrap),
    onSettled: () =>
      queryClient.invalidateQueries({ queryKey: contextSourceKeys.all }),
  })
}
