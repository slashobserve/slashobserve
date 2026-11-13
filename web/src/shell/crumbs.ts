import { useMatches } from "@tanstack/react-router"

import type { NavPath } from "./nav"

export type Crumb = { label: string; to?: NavPath | "/ask" | "/import" }

declare module "@tanstack/react-router" {
  interface StaticDataRouteOption {
    crumbs?: Crumb[]
  }
}

export function useCrumbs(): Crumb[] {
  return useMatches({
    select: (matches): Crumb[] =>
      matches.findLast((match) => match.staticData.crumbs)?.staticData.crumbs ??
      [],
  })
}
