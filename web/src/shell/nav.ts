import { roleAtLeast, type Role } from "@/lib/roles"

export type NavPath =
  "/" | "/dashboards" | "/datasources" | "/keys" | "/people" | "/settings"

export type NavItem = {
  to: NavPath
  label: string
  group: "main" | "workspace"
  minRole: Role
}

const navItems: readonly NavItem[] = [
  { to: "/", label: "Observe", group: "main", minRole: "viewer" },
  { to: "/dashboards", label: "Dashboards", group: "main", minRole: "viewer" },
  {
    to: "/datasources",
    label: "Datasources",
    group: "workspace",
    minRole: "admin",
  },
  { to: "/keys", label: "API keys", group: "workspace", minRole: "admin" },
  { to: "/people", label: "People", group: "workspace", minRole: "admin" },
  { to: "/settings", label: "Settings", group: "workspace", minRole: "viewer" },
]

export function visibleNavItems(role: Role): NavItem[] {
  return navItems.filter((item) => roleAtLeast(role, item.minRole))
}

export function isNavItemActive(to: NavPath, pathname: string): boolean {
  if (to === "/") {
    return (
      pathname === "/" ||
      pathname === "/import" ||
      pathname.startsWith("/observe/")
    )
  }
  return pathname === to || pathname.startsWith(`${to}/`)
}
