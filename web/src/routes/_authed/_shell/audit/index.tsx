import { createFileRoute } from "@tanstack/react-router"

import { AuditLog } from "@/audit/audit-log"
import {
  auditSearchOf,
  auditViewOf,
  readAuditSearch,
  type AuditSearch,
} from "@/audit/search"
import { RoleGate } from "@/components/role-gate"
import { useDocumentTitle } from "@/lib/document-title"

export const Route = createFileRoute("/_authed/_shell/audit/")({
  staticData: { crumbs: [{ label: "Audit" }] },
  validateSearch: (search: Record<string, unknown>): AuditSearch =>
    readAuditSearch(search),
  component: AuditLogRoute,
})

function AuditLogRoute() {
  useDocumentTitle("Audit log - SlashObserve")
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  const view = auditViewOf(search)
  return (
    <RoleGate min="admin">
      <AuditLog
        view={view}
        onView={(next) =>
          void navigate({
            search: (prev) => ({ ...prev, ...auditSearchOf(next) }),
            replace:
              next.q === view.q &&
              next.from === view.from &&
              next.to === view.to,
          })
        }
      />
    </RoleGate>
  )
}
