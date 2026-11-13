import { createContext, useContext, useLayoutEffect } from "react"
import { useMatches } from "@tanstack/react-router"

import type { NavPath } from "./nav"

export type Crumb = { label: string; to?: NavPath | "/ask" | "/import" }

declare module "@tanstack/react-router" {
  interface StaticDataRouteOption {
    crumbs?: Crumb[]
    pageTitle?: boolean
  }
}

type RouteCrumbs = { crumbs: Crumb[]; pageTitle: boolean }

export function useCrumbs(): RouteCrumbs {
  return useMatches({
    select: (matches): RouteCrumbs => {
      const match = matches.findLast((candidate) => candidate.staticData.crumbs)
      return {
        crumbs: match?.staticData.crumbs ?? [],
        pageTitle: match?.staticData.pageTitle ?? false,
      }
    },
  })
}

export const HideCrumbContext = createContext<(hidden: boolean) => void>(
  () => undefined
)

export function useHideCrumbWhenHeadingRepeats(title: string) {
  const { crumbs, pageTitle } = useCrumbs()
  const setHidden = useContext(HideCrumbContext)
  const repeats =
    !pageTitle && crumbs.length === 1 && crumbs[0]?.label === title
  useLayoutEffect(() => {
    if (!repeats) return
    setHidden(true)
    return () => setHidden(false)
  }, [repeats, setHidden])
}
