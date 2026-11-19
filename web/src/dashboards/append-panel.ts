import {
  allPanels,
  isRowPanel,
  type Dashboard,
  type DataPanel,
  type Panel,
} from "@/spec"

type PanelSlot = { id: string; y: number }

export type PanelBuilder = (slot: PanelSlot) => DataPanel

export function freePanelId(spec: Dashboard, base: string): string {
  const used = new Set(allPanels(spec).map((entry) => entry.panel.id))
  if (!used.has(base)) return base
  let n = 2
  while (used.has(`${base}-${n}`)) n++
  return `${base}-${n}`
}

function bottom(panels: readonly { gridPos: { y: number; h: number } }[]) {
  return panels.reduce(
    (most, panel) => Math.max(most, panel.gridPos.y + panel.gridPos.h),
    0
  )
}

export function gridBottom(spec: Dashboard): number {
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

export function appendPanel(
  spec: Dashboard,
  idBase: string,
  build: PanelBuilder
): Dashboard {
  const id = freePanelId(spec, idBase)
  const last = lastPanel(spec.panels)
  if (last !== undefined && isRowPanel(last) && last.collapsed) {
    const children = last.panels ?? []
    const y = children.length > 0 ? bottom(children) : last.gridPos.y + 1
    const row = { ...last, panels: [...children, build({ id, y })] }
    return {
      ...spec,
      panels: spec.panels.map((panel) => (panel === last ? row : panel)),
    }
  }
  return {
    ...spec,
    panels: [...spec.panels, build({ id, y: gridBottom(spec) })],
  }
}
