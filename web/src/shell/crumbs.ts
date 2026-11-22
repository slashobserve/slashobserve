import { createContext, useContext, useLayoutEffect } from "react"
import { useMatches } from "@tanstack/react-router"

import type { NavPath } from "./nav"

export type Crumb = { label: string; to?: NavPath | "/ask" | "/import" }

type CrumbParams = Record<string, string | undefined>

declare module "@tanstack/react-router" {
  interface StaticDataRouteOption {
    crumbs?: Crumb[] | ((params: CrumbParams) => Crumb[])
    pageTitle?: boolean
  }
}

type RouteCrumbs = { crumbs: Crumb[]; pageTitle: boolean }

export function useCrumbs(): RouteCrumbs {
  return useMatches({
    select: (matches): RouteCrumbs => {
      const match = matches.findLast((candidate) => candidate.staticData.crumbs)
      const crumbs = match?.staticData.crumbs
      return {
        crumbs:
          typeof crumbs === "function" ? crumbs(match!.params) : (crumbs ?? []),
        pageTitle: match?.staticData.pageTitle ?? false,
      }
    },
  })
}

export const HideCrumbContext = createContext<(hidden: boolean) => void>(
  () => undefined
)

export function useHideCrumbWhenHeadingRepeats(title: string): boolean {
  const { crumbs, pageTitle } = useCrumbs()
  const setHidden = useContext(HideCrumbContext)
  const repeats =
    !pageTitle && crumbs.length === 1 && crumbs[0]?.label === title
  useLayoutEffect(() => {
    if (!repeats) return
    setHidden(true)
    return () => setHidden(false)
  }, [repeats, setHidden])
  return repeats
}
