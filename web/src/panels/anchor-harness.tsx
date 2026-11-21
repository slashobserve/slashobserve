import type { ReactNode } from "react"
import { render } from "@testing-library/react"

import type { PanelDataMeta } from "@/dashboards/queries"
import { displayWithDefaults, type DataPanel } from "@/spec"

import { AnchorLayoutProvider, createAnchorLayoutStore } from "./anchor-layout"
import { createFieldResolver } from "./field-config"
import { createSharedCursor } from "./shared-cursor"
import type { PreparedFrame } from "./transforms"
import type { PanelRenderer, PanelRendererProps } from "./types"

export const meta = {
  from: "2026-11-21T10:00:00Z",
  to: "2026-11-21T11:00:00Z",
} as PanelDataMeta

export function panelOf(
  type: DataPanel["type"],
  display: Record<string, unknown> = {}
): DataPanel {
  return {
    id: "p1",
    title: "Panel",
    gridPos: { x: 0, y: 0, w: 12, h: 8 },
    type,
    datasource: "ds",
    queries: [{ query: "q" }],
    display,
  } as DataPanel
}

export function renderInStore(ui: ReactNode) {
  const store = createAnchorLayoutStore()
  const view = render(
    <AnchorLayoutProvider value={store}>{ui}</AnchorLayoutProvider>
  )
  return { store, view }
}

export function renderPublishing(
  Renderer: PanelRenderer,
  panel: DataPanel,
  frames: PreparedFrame[],
  extra: Partial<PanelRendererProps> = {}
) {
  const props: PanelRendererProps = {
    panel,
    frames,
    fields: createFieldResolver(panel),
    display: displayWithDefaults(
      panel.type as Parameters<typeof displayWithDefaults>[0],
      panel.display
    ),
    zone: "UTC",
    meta,
    width: 400,
    height: 240,
    stateScope: "test",
    cursor: createSharedCursor(),
    onTimeRangeSelect: () => undefined,
    links: { vars: {} },
    ...extra,
  }
  return renderInStore(<Renderer {...props} />)
}
