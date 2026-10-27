import type {
  Dashboard,
  DataPanel,
  Panel,
  PanelQuery,
  RowPanel,
} from "./observespec.gen"

export type PanelType = Panel["type"]
export type DataPanelType = DataPanel["type"]

export const dataPanelTypes: readonly DataPanelType[] = [
  "timeseries",
  "stat",
  "gauge",
  "bargauge",
  "table",
  "piechart",
  "heatmap",
  "histogram",
  "state-timeline",
  "logs",
  "barchart",
  "status-history",
  "trend",
  "xychart",
  "candlestick",
  "nodeGraph",
  "traces",
  "flamegraph",
  "geomap",
  "canvas",
  "alertlist",
]

export function isDataPanel(panel: Panel): panel is DataPanel {
  return (dataPanelTypes as readonly string[]).includes(panel.type)
}

export function isRowPanel(panel: Panel): panel is RowPanel {
  return panel.type === "row"
}

export type LayoutPanel = Exclude<Panel, RowPanel>

export function panelTitle(panel: { id: string; title?: string }): string {
  return panel.title ?? panel.id
}

export type PanelEntry = { panel: Panel; pointer: string; row?: RowPanel }

export function allPanels(spec: Dashboard): PanelEntry[] {
  const entries: PanelEntry[] = []
  spec.panels.forEach((panel, i) => {
    entries.push({ panel, pointer: `/panels/${i}` })
    if (isRowPanel(panel)) {
      panel.panels?.forEach((child, j) => {
        entries.push({
          panel: child,
          pointer: `/panels/${i}/panels/${j}`,
          row: panel,
        })
      })
    }
  })
  return entries
}

export function findPanel(spec: Dashboard, id: string): PanelEntry | undefined {
  return allPanels(spec).find((entry) => entry.panel.id === id)
}

export function queryRef(query: PanelQuery, index: number): string {
  return query.ref ?? String.fromCharCode(65 + index)
}
