import type { RefObject } from "react"

import { AddPanelToDashboard } from "@/dashboards/add-panel-to-dashboard"

import { logsPanelFor } from "./add-panel"

type Props = {
  datasource: string
  query: string
  onClose: () => void
  trigger: RefObject<HTMLElement | null>
}

export function AddToDashboardPanel({
  datasource,
  query,
  onClose,
  trigger,
}: Props) {
  return (
    <AddPanelToDashboard
      idBase="logs"
      defaultTitle="Logs"
      description="Adds a full-width logs panel with this query under the dashboard's last row."
      summary="Add a logs panel from the logs page"
      panelFor={(slot) => logsPanelFor({ ...slot, datasource, query })}
      onClose={onClose}
      trigger={trigger}
    />
  )
}
