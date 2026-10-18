import type { ComponentType } from "react"

import type { PanelDataMeta, WireFrame } from "@/dashboards/queries"
import type { DataPanel, DataPanelType } from "@/spec"

export type PanelRendererProps = {
  panel: DataPanel
  frames: WireFrame[]
  meta: PanelDataMeta
  width: number
  height: number
  syncKey: string
  onTimeRangeSelect: (fromMs: number, toMs: number) => void
}

export type PanelRenderer = ComponentType<PanelRendererProps>

export type PanelRegistry = Partial<Record<DataPanelType, PanelRenderer>>
