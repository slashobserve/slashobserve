import {
  queryOptions,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { dashboardKeys, folderKeys } from "@/api/keys"
import type { components } from "@/api/schema.gen"

import type { PutEntry } from "./grants"

export type Folder = components["schemas"]["Folder"]
export type FolderPermissions = components["schemas"]["FolderPermissions"]

export function folderQuery(id: string) {
  return queryOptions({
    queryKey: folderKeys.detail(id),
    queryFn: ({ signal }) =>
      api
        .GET("/folders/{id}", { params: { path: { id } }, signal })
        .then(unwrap),
  })
}

export function folderPermissionsQuery(id: string) {
  return queryOptions({
    queryKey: folderKeys.permissions(id),
    queryFn: ({ signal }) =>
      api
        .GET("/folders/{id}/permissions", { params: { path: { id } }, signal })
        .then(unwrap),
  })
}

export function useRenameFolder(id: string) {
  const queryClient = useQueryClient()
  return useMutation({
    networkMode: "always",
    mutationFn: (title: string) =>
      api
        .PATCH("/folders/{id}", { params: { path: { id } }, body: { title } })
        .then(unwrap),
    onSettled: () =>
      Promise.all([
        queryClient.invalidateQueries({ queryKey: folderKeys.all }),
        queryClient.invalidateQueries({ queryKey: dashboardKeys.all }),
      ]),
  })
}

export function useDeleteFolder(id: string) {
  const queryClient = useQueryClient()
  return useMutation({
    networkMode: "always",
    mutationFn: () =>
      api.DELETE("/folders/{id}", { params: { path: { id } } }).then(unwrap),
    onSuccess: () =>
      queryClient.removeQueries({ queryKey: folderKeys.detail(id) }),
    onSettled: () =>
      Promise.all([
        queryClient.invalidateQueries({ queryKey: folderKeys.all }),
        queryClient.invalidateQueries({ queryKey: dashboardKeys.list() }),
      ]),
  })
}

export function useSetFolderPermissions(id: string) {
  const queryClient = useQueryClient()
  return useMutation({
    networkMode: "always",
    mutationFn: (entries: PutEntry[]) =>
      api
        .PUT("/folders/{id}/permissions", {
          params: { path: { id } },
          body: { entries },
        })
        .then(unwrap),
    onSettled: () =>
      Promise.all([
        queryClient.invalidateQueries({ queryKey: folderKeys.all }),
        queryClient.invalidateQueries({ queryKey: dashboardKeys.all }),
      ]),
  })
}
