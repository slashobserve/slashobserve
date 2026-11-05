import { Link, useRouteContext } from "@tanstack/react-router"

import { roleAtLeast } from "@/lib/roles"

const tabClass =
  "relative inline-flex h-8 items-center rounded-control px-2.5 text-sm font-medium text-muted-foreground outline-none after:absolute after:inset-x-0 after:-bottom-px after:h-0.5 after:bg-foreground after:opacity-0 hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"

const activeProps = {
  className: "text-foreground after:opacity-100",
  "aria-current": "page",
} as const

export function SettingsTabs() {
  const { principal } = useRouteContext({ from: "/_authed" })
  return (
    <nav aria-label="Settings sections" className="mb-4 flex gap-1 border-b">
      <Link
        to="/settings"
        activeOptions={{ exact: true }}
        activeProps={activeProps}
        className={tabClass}
      >
        General
      </Link>
      {roleAtLeast(principal.role, "admin") ? (
        <Link
          to="/settings/grafana"
          activeOptions={{ exact: true }}
          activeProps={activeProps}
          className={tabClass}
        >
          Grafana
        </Link>
      ) : null}
    </nav>
  )
}
