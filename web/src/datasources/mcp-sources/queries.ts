import {
  queryOptions,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { mcpSourceKeys } from "@/api/keys"
import type { components } from "@/api/schema.gen"

export type MCPSource = components["schemas"]["MCPSource"]
export type MCPTool = components["schemas"]["MCPSourceTool"]
export type CreateMCPSourceBody =
  components["schemas"]["CreateMCPSourceRequest"]
export type UpdateMCPSourceBody =
  components["schemas"]["UpdateMCPSourceRequest"]

export function mcpSourcesQuery() {
  return queryOptions({
    queryKey: mcpSourceKeys.list(),
    queryFn: ({ signal }) =>
      api
        .GET("/mcp-sources", { signal })
        .then(unwrap)
        .then((body) => body.mcp_sources),
  })
}

export function mcpToolsQuery(uid: string) {
  return queryOptions({
    queryKey: mcpSourceKeys.tools(uid),
    queryFn: ({ signal }) =>
      api
        .GET("/mcp-sources/{uid}/tools", {
          params: { path: { uid } },
          signal,
        })
        .then(unwrap)
        .then((body) => body.tools),
  })
}

export function useCreateMCPSource() {
  const queryClient = useQueryClient()
  return useMutation({
    gcTime: 0,
    networkMode: "always",
    mutationFn: (body: CreateMCPSourceBody) =>
      api.POST("/mcp-sources", { body }).then(unwrap),
    onSettled: () =>
      queryClient.invalidateQueries({ queryKey: mcpSourceKeys.all }),
  })
}

export function useUpdateMCPSource() {
  const queryClient = useQueryClient()
  return useMutation({
    gcTime: 0,
    networkMode: "always",
    mutationFn: ({ uid, body }: { uid: string; body: UpdateMCPSourceBody }) =>
      api
        .PATCH("/mcp-sources/{uid}", { params: { path: { uid } }, body })
        .then(unwrap),
    onSettled: () =>
      queryClient.invalidateQueries({ queryKey: mcpSourceKeys.all }),
  })
}

export function useDeleteMCPSource() {
  const queryClient = useQueryClient()
  return useMutation({
    networkMode: "always",
    mutationFn: (uid: string) =>
      api
        .DELETE("/mcp-sources/{uid}", { params: { path: { uid } } })
        .then(unwrap),
    onSettled: () =>
      queryClient.invalidateQueries({ queryKey: mcpSourceKeys.all }),
  })
}

export function useDiscoverMCPSource() {
  const queryClient = useQueryClient()
  return useMutation({
    networkMode: "always",
    mutationFn: (uid: string) =>
      api
        .POST("/mcp-sources/{uid}/discover", { params: { path: { uid } } })
        .then(unwrap),
    onSettled: () =>
      queryClient.invalidateQueries({ queryKey: mcpSourceKeys.all }),
  })
}

export function useApproveMCPTool() {
  const queryClient = useQueryClient()
  return useMutation({
    networkMode: "always",
    mutationFn: ({
      uid,
      tool,
    }: {
      uid: string
      tool: Pick<MCPTool, "name" | "hash" | "seen">
    }) =>
      api
        .POST("/mcp-sources/{uid}/tools/{name}/approve", {
          params: { path: { uid, name: tool.name } },
          body: { hash: tool.seen?.hash ?? tool.hash },
        })
        .then(unwrap),
    onSettled: () =>
      queryClient.invalidateQueries({ queryKey: mcpSourceKeys.all }),
  })
}

export function useApproveReadOnlyTools() {
  const queryClient = useQueryClient()
  return useMutation({
    networkMode: "always",
    mutationFn: (uid: string) =>
      api
        .POST("/mcp-sources/{uid}/tools/approve-read-only", {
          params: { path: { uid } },
        })
        .then(unwrap),
    onSettled: () =>
      queryClient.invalidateQueries({ queryKey: mcpSourceKeys.all }),
  })
}

export function useRevokeMCPTool() {
  const queryClient = useQueryClient()
  return useMutation({
    networkMode: "always",
    mutationFn: ({ uid, name }: { uid: string; name: string }) =>
      api
        .POST("/mcp-sources/{uid}/tools/{name}/revoke", {
          params: { path: { uid, name } },
        })
        .then(unwrap),
    onSettled: () =>
      queryClient.invalidateQueries({ queryKey: mcpSourceKeys.all }),
  })
}
