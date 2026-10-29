import type { Role } from "@/lib/roles"

import { visibleNavItems, type NavPath } from "./nav"

type PageEntry = {
  kind: "page"
  id: string
  label: string
  to: NavPath
}
export type DashboardEntry = { kind: "dashboard"; id: string; label: string }
type ActionEntry = {
  kind: "action"
  id: "action:sign-out"
  label: string
}
export type PaletteEntry = PageEntry | DashboardEntry | ActionEntry
export type PaletteGroup = {
  value: "Pages" | "Dashboards" | "Account"
  items: PaletteEntry[]
}

function pageEntries(role: Role): PageEntry[] {
  return visibleNavItems(role).map((item) => ({
    kind: "page",
    id: `page:${item.to}`,
    label: item.label,
    to: item.to,
  }))
}

const signOutEntry: ActionEntry = {
  kind: "action",
  id: "action:sign-out",
  label: "Sign out",
}

const dashboardLimit = 20
const dashboardLimitWithoutQuery = 8

export function filterPalette(
  query: string,
  dashboards: { id: string; title: string }[],
  role: Role
): PaletteGroup[] {
  const needle = query.trim().toLowerCase()
  const matches = (label: string) => label.toLowerCase().includes(needle)
  const dashboardEntries: DashboardEntry[] = dashboards
    .filter((dashboard) => matches(dashboard.title))
    .slice(0, needle === "" ? dashboardLimitWithoutQuery : dashboardLimit)
    .map((dashboard) => ({
      kind: "dashboard",
      id: dashboard.id,
      label: dashboard.title,
    }))
  const groups: PaletteGroup[] = [
    {
      value: "Pages",
      items: pageEntries(role).filter((e) => matches(e.label)),
    },
    { value: "Dashboards", items: dashboardEntries },
    {
      value: "Account",
      items: matches(signOutEntry.label) ? [signOutEntry] : [],
    },
  ]
  return groups.filter((group) => group.items.length > 0)
}
