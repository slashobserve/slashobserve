import { createFileRoute } from "@tanstack/react-router"

import { auditPageTitles } from "@/audit/audit-folder"
import { AuditObservations } from "@/audit/audit-observations"
import { RoleGate } from "@/components/role-gate"
import { useDocumentTitle } from "@/lib/document-title"

export const Route = createFileRoute("/_authed/_shell/audit/observations")({
  staticData: {
    crumbs: [{ label: "Audit", to: "/audit" }, { label: "Observations" }],
  },
  component: AuditObservationsRoute,
})

function AuditObservationsRoute() {
  useDocumentTitle(auditPageTitles.observations)
  return (
    <RoleGate min="admin" hideCrumb>
      <AuditObservations />
    </RoleGate>
  )
}
