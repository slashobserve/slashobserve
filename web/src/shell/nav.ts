import { roleAtLeast, type Role } from "@/lib/roles"

export type NavPath =
  | "/"
  | "/inbox"
  | "/observations"
  | "/dashboards"
  | "/logs"
  | "/traces"
  | "/agents"
  | "/datasources"
  | "/people"
  | "/settings"

export type NavItem = {
  to: NavPath
  label: string
  group: "main" | "workspace"
  minRole: Role
}

const navItems: readonly NavItem[] = [
  { to: "/", label: "Home", group: "main", minRole: "viewer" },
  { to: "/inbox", label: "Inbox", group: "main", minRole: "viewer" },
  {
    to: "/observations",
    label: "Observations",
    group: "main",
    minRole: "viewer",
  },
  { to: "/dashboards", label: "Dashboards", group: "main", minRole: "viewer" },
  { to: "/logs", label: "Logs", group: "main", minRole: "viewer" },
  { to: "/traces", label: "Traces", group: "main", minRole: "viewer" },
  { to: "/agents", label: "Agents", group: "main", minRole: "viewer" },
  {
    to: "/datasources",
    label: "Datasources",
    group: "workspace",
    minRole: "admin",
  },
  { to: "/people", label: "People", group: "workspace", minRole: "admin" },
  { to: "/settings", label: "Settings", group: "workspace", minRole: "viewer" },
]

export function visibleNavItems(role: Role): NavItem[] {
  return navItems.filter((item) => roleAtLeast(role, item.minRole))
}

export function isNavItemActive(to: NavPath, pathname: string): boolean {
  if (to === "/") return pathname === "/"
  if (to === "/traces" && pathname.startsWith("/t/")) return true
  if (to === "/agents" && pathname === "/keys") return true
  if (to === "/observations" && pathname.startsWith("/o/")) return true
  return pathname === to || pathname.startsWith(`${to}/`)
}

export type SubNavRow = {
  id: string
  paths: readonly string[]
}

export type ParentState = "current" | "contains-current" | "none"

export function subNavState(
  item: NavItem,
  rows: readonly SubNavRow[],
  pathname: string
): { parent: ParentState; currentId: string | null } {
  const current = rows.find((row) => row.paths.includes(pathname))
  if (current !== undefined) {
    return { parent: "contains-current", currentId: current.id }
  }
  return {
    parent: isNavItemActive(item.to, pathname) ? "current" : "none",
    currentId: null,
  }
}

export function unreadBadge(count: number | null): string | null {
  if (count === null || count <= 0) return null
  return count >= 100 ? "99+" : String(count)
}

export function countLabel(shown: number, more: boolean): string | null {
  if (shown <= 0) return null
  return more ? `${shown}+` : String(shown)
}
