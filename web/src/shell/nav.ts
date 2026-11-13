import { roleAtLeast, type Role } from "@/lib/roles"

export type NavPath =
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
  if (to === "/traces" && pathname.startsWith("/t/")) return true
  if (to === "/agents" && pathname === "/keys") return true
  if (to === "/observations" && pathname.startsWith("/o/")) return true
  return pathname === to || pathname.startsWith(`${to}/`)
}

export function unreadBadge(count: number | null): string | null {
  if (count === null || count <= 0) return null
  return count >= 100 ? "99+" : String(count)
}
