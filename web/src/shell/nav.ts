export type NavPath =
  "/" | "/dashboards" | "/datasources" | "/keys" | "/people" | "/settings"

export type NavItem = {
  to: NavPath
  label: string
  group: "main" | "workspace"
}

export const navItems: readonly NavItem[] = [
  { to: "/", label: "Observe", group: "main" },
  { to: "/dashboards", label: "Dashboards", group: "main" },
  { to: "/datasources", label: "Datasources", group: "workspace" },
  { to: "/keys", label: "API keys", group: "workspace" },
  { to: "/people", label: "People", group: "workspace" },
  { to: "/settings", label: "Settings", group: "workspace" },
]

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
