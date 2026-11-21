import { Link } from "@tanstack/react-router"

import { auditDatasourceUid } from "@/audit/audit-source"
import { DropdownMenuLinkItem } from "@/components/ui/dropdown-menu"
import { logsLink, logsPath } from "@/lib/links"

import type { SubNavRow } from "./nav"
import { railFlyoutItemClass } from "./rail-flyout"
import { SubNavList, subNavLinkClass } from "./sub-nav"

const pages = [
  { id: "log", label: "Log", path: "/audit" },
  { id: "dashboards", label: "Folder", path: "/audit/dashboards" },
  { id: "explore", label: "Explore", path: logsPath(auditDatasourceUid) },
] as const

type AuditPage = (typeof pages)[number]

export const auditRows: readonly SubNavRow[] = pages.map(({ id, path }) => ({
  id,
  paths: [path],
}))

function targetOf(page: AuditPage) {
  switch (page.id) {
    case "log":
      return { to: "/audit" } as const
    case "dashboards":
      return { to: "/audit/dashboards" } as const
    case "explore":
      return logsLink(auditDatasourceUid)
  }
}

export function AuditList({
  currentId,
  onNavigate,
}: {
  currentId: string | null
  onNavigate?: () => void
}) {
  return (
    <SubNavList label="Audit">
      {pages.map((page) => (
        <li key={page.id}>
          <Link
            {...targetOf(page)}
            activeOptions={{ exact: true }}
            onClick={onNavigate}
            aria-current={page.id === currentId ? "page" : undefined}
            className={subNavLinkClass}
          >
            <span className="truncate">{page.label}</span>
          </Link>
        </li>
      ))}
    </SubNavList>
  )
}

export function AuditFlyoutItems() {
  return (
    <>
      {pages.map((page) => (
        <DropdownMenuLinkItem
          key={page.id}
          render={<Link {...targetOf(page)} />}
          className={railFlyoutItemClass}
        >
          {page.label}
        </DropdownMenuLinkItem>
      ))}
    </>
  )
}
