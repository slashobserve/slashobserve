import { findPanel, isRowPanel, type Dashboard } from "@/spec"

export function withResponsive(
  spec: Dashboard,
  panelId: string,
  responsive: boolean
): Dashboard {
  const next = structuredClone(spec)
  const found = findPanel(next, panelId)
  if (found === undefined || isRowPanel(found.panel)) return next
  if (responsive) delete found.panel.responsive
  else found.panel.responsive = false
  return next
}
