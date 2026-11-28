import { logsLink } from "@/lib/links"
import { validQuery } from "@/logs/search"

import { auditDatasourceUid } from "./audit-source"
import type { AuditView } from "./search"

export function auditInLogsLink(view: Pick<AuditView, "q" | "from" | "to">) {
  const q = validQuery(view.q)
  return logsLink(auditDatasourceUid, {
    ...(q === undefined ? {} : { q }),
    from: view.from,
    to: view.to,
  })
}
