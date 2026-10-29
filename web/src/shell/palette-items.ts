import { navItems, type NavPath } from "./nav"

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

const pageEntries: PageEntry[] = navItems.map((item) => ({
  kind: "page",
  id: `page:${item.to}`,
  label: item.label,
  to: item.to,
}))

const signOutEntry: ActionEntry = {
  kind: "action",
  id: "action:sign-out",
  label: "Sign out",
}

const dashboardLimit = 20
const dashboardLimitWithoutQuery = 8

export function filterPalette(
  query: string,
  dashboards: { id: string; title: string }[]
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
    { value: "Pages", items: pageEntries.filter((e) => matches(e.label)) },
    { value: "Dashboards", items: dashboardEntries },
    {
      value: "Account",
      items: matches(signOutEntry.label) ? [signOutEntry] : [],
    },
  ]
  return groups.filter((group) => group.items.length > 0)
}

export function shortcutLabel(userAgent: string): string {
  return /Mac|iPhone|iPad|iPod/.test(userAgent) ? "⌘K" : "Ctrl K"
}

export function isPaletteShortcut(event: {
  key: string
  metaKey: boolean
  ctrlKey: boolean
  altKey: boolean
  shiftKey: boolean
}): boolean {
  return (
    (event.metaKey || event.ctrlKey) &&
    !event.altKey &&
    !event.shiftKey &&
    event.key.toLowerCase() === "k"
  )
}
