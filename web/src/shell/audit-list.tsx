import { Link } from "@tanstack/react-router"

import { auditDatasourceUid } from "@/audit/audit-source"
import { DropdownMenuLinkItem } from "@/components/ui/dropdown-menu"
import { logsPath } from "@/lib/links"

import type { SubNavRow } from "./nav"
import { railFlyoutItemClass } from "./rail-flyout"
import { SubNavList, subNavLinkClass } from "./sub-nav"

const pages = [
  {
    id: "log",
    label: "Log",
    to: "/audit",
    paths: ["/audit", logsPath(auditDatasourceUid)],
  },
  {
    id: "dashboards",
    label: "Dashboards",
    to: "/audit/dashboards",
    paths: ["/audit/dashboards"],
  },
  {
    id: "observations",
    label: "Observations",
    to: "/audit/observations",
    paths: ["/audit/observations"],
  },
] as const

export const auditRows: readonly SubNavRow[] = pages.map(({ id, paths }) => ({
  id,
  paths,
}))

export function AuditList({
  currentId,
  onNavigate,
}: {
  currentId: string | null
  onNavigate?: (() => void) | undefined
}) {
  return (
    <SubNavList label="Audit">
      {pages.map((page) => (
        <li key={page.id}>
          <Link
            to={page.to}
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
          render={<Link to={page.to} />}
          className={railFlyoutItemClass}
        >
          {page.label}
        </DropdownMenuLinkItem>
      ))}
    </>
  )
}
