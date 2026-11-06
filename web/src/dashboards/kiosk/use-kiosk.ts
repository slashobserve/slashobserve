import { useEffect, useRef, type RefObject } from "react"
import { useQuery } from "@tanstack/react-query"
import { useNavigate } from "@tanstack/react-router"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { dashboardKeys } from "@/api/keys"
import { dashboardLink } from "@/lib/links"

import { cycleInterval, searchFromState, type DashboardSearch } from "../search"
import { nextFolderDashboard, nextRowIndex } from "./kiosk"

const folderCycleLimit = 100

type Args = {
  dashboardId: string
  folderId: string
  search: DashboardSearch
  grid: RefObject<HTMLElement | null>
}

function scrollBehavior(): ScrollBehavior {
  return window.matchMedia("(prefers-reduced-motion: reduce)").matches
    ? "auto"
    : "smooth"
}

export function useKiosk({ dashboardId, folderId, search, grid }: Args) {
  const cycle = search.kiosk ? search.cycle : undefined
  const periodMs = (search.interval ?? cycleInterval.initial) * 1000
  useRowCycle(cycle === "rows", periodMs, grid)
  useFolderCycle(cycle === "folder", periodMs, dashboardId, folderId, search)
}

function useRowCycle(
  active: boolean,
  periodMs: number,
  grid: RefObject<HTMLElement | null>
) {
  useEffect(() => {
    if (!active) return
    let current: number | null = null
    const timer = window.setInterval(() => {
      const headers =
        grid.current?.querySelectorAll<HTMLElement>(
          '[data-row-id][data-collapsed="false"]'
        ) ?? []
      current = nextRowIndex(headers.length, current)
      if (current === null) return
      headers[current].scrollIntoView({
        block: "start",
        behavior: scrollBehavior(),
      })
    }, periodMs)
    return () => window.clearInterval(timer)
  }, [active, periodMs, grid])
}

function useFolderCycle(
  active: boolean,
  periodMs: number,
  dashboardId: string,
  folderId: string,
  search: DashboardSearch
) {
  const navigate = useNavigate()
  const listed = useQuery({
    queryKey: dashboardKeys.inFolder(folderId),
    queryFn: ({ signal }) =>
      api
        .GET("/dashboards", {
          params: { query: { folder: folderId, limit: folderCycleLimit } },
          signal,
        })
        .then(unwrap)
        .then((page) => page.dashboards),
    enabled: active,
    staleTime: 60_000,
  })
  const next = active
    ? nextFolderDashboard(listed.data ?? [], dashboardId)
    : null
  const nextId = next?.id
  const nextTitle = next?.title
  const carried = useRef(search)
  useEffect(() => {
    carried.current = search
  })
  useEffect(() => {
    if (nextId === undefined) return
    const timer = window.setTimeout(() => {
      const { from, to, refresh, interval } = carried.current
      void navigate({
        ...dashboardLink(nextId, nextTitle),
        search: searchFromState({
          kiosk: true,
          cycle: "folder",
          interval,
          from,
          to,
          refresh,
          vars: {},
        }),
      })
    }, periodMs)
    return () => window.clearTimeout(timer)
  }, [nextId, nextTitle, periodMs, navigate])
}
