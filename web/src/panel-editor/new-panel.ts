import { create } from "zustand"

import { freePanelId, gridBottom } from "@/dashboards/append-panel"
import { allPanels, isDataPanel, type Dashboard, type DataPanel } from "@/spec"

const newPanelSize = { w: 12, h: 8 }

function dashboardDatasource(spec: Dashboard): string | undefined {
  for (const { panel } of allPanels(spec)) {
    if (isDataPanel(panel)) return panel.datasource
  }
  return undefined
}

export function newPanelDraft(
  spec: Dashboard,
  datasources: readonly { uid: string }[]
): DataPanel | null {
  const datasource = dashboardDatasource(spec) ?? datasources[0]?.uid
  if (datasource === undefined) return null
  return {
    id: freePanelId(spec, "panel"),
    type: "timeseries",
    title: "New panel",
    datasource,
    gridPos: { x: 0, y: gridBottom(spec), ...newPanelSize },
    queries: [{ query: "" }],
  }
}

export function withNewPanel(spec: Dashboard, panel: DataPanel): Dashboard {
  return {
    ...spec,
    panels: [
      ...spec.panels,
      { ...panel, gridPos: { ...panel.gridPos, y: gridBottom(spec) } },
    ],
  }
}

type NewPanels = {
  panels: Record<string, DataPanel>
  open: (dashboardId: string, panel: DataPanel) => void
}

export const useNewPanels = create<NewPanels>()((set) => ({
  panels: {},
  open: (dashboardId, panel) =>
    set((state) => ({ panels: { ...state.panels, [dashboardId]: panel } })),
}))
