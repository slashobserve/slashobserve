import {
  allPanels,
  isRowPanel,
  type Dashboard,
  type DataPanel,
  type Panel,
} from "@/spec"

type LogsPanelInput = { title: string; datasource: string; query: string }

export function freePanelId(spec: Dashboard, base = "logs"): string {
  const used = new Set(allPanels(spec).map((entry) => entry.panel.id))
  if (!used.has(base)) return base
  let n = 2
  while (used.has(`${base}-${n}`)) n++
  return `${base}-${n}`
}

function logsPanelFor({
  id,
  title,
  datasource,
  query,
  y,
}: LogsPanelInput & { id: string; y: number }): DataPanel {
  return {
    id,
    type: "logs",
    title,
    datasource,
    gridPos: { x: 0, y, w: 24, h: 10 },
    queries: [{ query }],
    display: { showTime: true },
  }
}

function bottom(panels: readonly { gridPos: { y: number; h: number } }[]) {
  return panels.reduce(
    (most, panel) => Math.max(most, panel.gridPos.y + panel.gridPos.h),
    0
  )
}

function gridBottom(spec: Dashboard): number {
  return bottom(
    allPanels(spec)
      .filter((entry) => !entry.row?.collapsed)
      .map((entry) => entry.panel)
  )
}

function lastPanel(panels: readonly Panel[]): Panel | undefined {
  let last: Panel | undefined
  for (const panel of panels) {
    if (last === undefined || panel.gridPos.y >= last.gridPos.y) last = panel
  }
  return last
}

export function appendLogsPanel(
  spec: Dashboard,
  input: LogsPanelInput
): Dashboard {
  const id = freePanelId(spec)
  const last = lastPanel(spec.panels)
  if (last !== undefined && isRowPanel(last) && last.collapsed) {
    const children = last.panels ?? []
    const y = children.length > 0 ? bottom(children) : last.gridPos.y + 1
    const row = {
      ...last,
      panels: [...children, logsPanelFor({ ...input, id, y })],
    }
    return {
      ...spec,
      panels: spec.panels.map((panel) => (panel === last ? row : panel)),
    }
  }
  return {
    ...spec,
    panels: [
      ...spec.panels,
      logsPanelFor({ ...input, id, y: gridBottom(spec) }),
    ],
  }
}
