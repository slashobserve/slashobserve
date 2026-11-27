import { useEffect, useState } from "react"
import {
  queryOptions,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query"

import { api } from "@/api/client"
import { ApiError, unwrap } from "@/api/errors"
import { publicKeys } from "@/api/keys"
import type { components } from "@/api/schema.gen"
import type { Dashboard } from "@/spec"

type PublicDashboardBody = components["schemas"]["PublicDashboard"]

export type PublicDashboard = Omit<PublicDashboardBody, "spec"> & {
  spec: Dashboard
}

function withoutQueryText(body: PublicDashboardBody): PublicDashboard {
  return { ...body, spec: body.spec as unknown as Dashboard }
}

export function publicDashboardQuery(token: string) {
  return queryOptions({
    queryKey: publicKeys.dashboard(token),
    queryFn: ({ signal }) =>
      api
        .GET("/public/{token}", { params: { path: { token } }, signal })
        .then(unwrap)
        .then(withoutQueryText),
    staleTime: 60_000,
  })
}

export function isLinkGoneError(
  queryKey: readonly unknown[],
  token: string,
  error: unknown
): boolean {
  return (
    queryKey[0] === "public" &&
    queryKey[1] === token &&
    queryKey.length > 2 &&
    error instanceof ApiError &&
    error.status === 404 &&
    error.code === "not_found"
  )
}

export function useLinkGone(token: string): boolean {
  const queryClient = useQueryClient()
  const [gone, setGone] = useState(false)
  useEffect(
    () =>
      queryClient.getQueryCache().subscribe((event) => {
        if (
          event.type === "updated" &&
          event.action.type === "error" &&
          isLinkGoneError(event.query.queryKey, token, event.action.error)
        ) {
          setGone(true)
        }
      }),
    [queryClient, token]
  )
  return gone
}

export function publicLinksQuery(dashboardId: string) {
  return queryOptions({
    queryKey: publicKeys.links(dashboardId),
    queryFn: ({ signal }) =>
      api
        .GET("/dashboards/{id}/public-links", {
          params: { path: { id: dashboardId } },
          signal,
        })
        .then(unwrap),
    select: (data) => data.public_links,
  })
}

type CreatePublicLink = components["schemas"]["CreatePublicLinkRequest"]

export function useCreatePublicLink(dashboardId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    gcTime: 0,
    mutationFn: (body: CreatePublicLink) =>
      api
        .POST("/dashboards/{id}/public-links", {
          params: { path: { id: dashboardId } },
          body,
        })
        .then(unwrap),
    onSuccess: () =>
      queryClient.invalidateQueries({
        queryKey: publicKeys.links(dashboardId),
      }),
  })
}

export function useRevokePublicLink(dashboardId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (linkId: string) =>
      api
        .DELETE("/dashboards/{id}/public-links/{link_id}", {
          params: { path: { id: dashboardId, link_id: linkId } },
        })
        .then(unwrap),
    onSettled: () =>
      queryClient.invalidateQueries({
        queryKey: publicKeys.links(dashboardId),
      }),
  })
}
