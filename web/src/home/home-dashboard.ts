import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient,
} from "@tanstack/react-query"
import { toast } from "sonner"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { homeDashboardKeys } from "@/api/keys"

const checkedKey = "observe.home-dashboard-checked"

function homeDashboardQuery() {
  return queryOptions({
    queryKey: homeDashboardKeys.current(),
    queryFn: ({ signal }) =>
      api.GET("/me/home-dashboard", { signal }).then(unwrap),
    staleTime: 30_000,
  })
}

function checked(): boolean {
  try {
    return sessionStorage.getItem(checkedKey) !== null
  } catch {
    return true
  }
}

export function markHomeChecked() {
  try {
    sessionStorage.setItem(checkedKey, "1")
  } catch {
    return
  }
}

export function forgetHomeChecked() {
  try {
    sessionStorage.removeItem(checkedKey)
  } catch {
    return
  }
}

export async function homeDashboardHref(
  queryClient: QueryClient,
  searchStr: string
): Promise<string | null> {
  if (searchStr !== "" || checked()) return null
  markHomeChecked()
  try {
    const home = await queryClient.fetchQuery(homeDashboardQuery())
    return home.status === "ok" && home.dashboard
      ? `/d/${home.dashboard.id}`
      : null
  } catch {
    return null
  }
}

export function useHomeDashboard() {
  return useQuery(homeDashboardQuery())
}

export function useSetHomeDashboard() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (dashboardId: string) =>
      api
        .PUT("/me/home-dashboard", { body: { dashboard_id: dashboardId } })
        .then(unwrap),
    onSuccess: (home) =>
      queryClient.setQueryData(homeDashboardKeys.current(), home),
    onError: () => toast.error("Could not set the home dashboard"),
  })
}

export function useClearHomeDashboard() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () => api.DELETE("/me/home-dashboard").then(unwrap),
    onSuccess: () =>
      queryClient.setQueryData(homeDashboardKeys.current(), {
        status: "none",
        dashboard: null,
        set_at: null,
      }),
    onError: () => toast.error("Could not clear the home dashboard"),
  })
}
