import type { NavPath } from "./nav"

export type Crumb = { label: string; to?: NavPath }

declare module "@tanstack/react-router" {
  interface StaticDataRouteOption {
    crumbs?: Crumb[]
  }
}
