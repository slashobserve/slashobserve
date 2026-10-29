import type { ComponentType } from "react"

import type { Selections } from "@/api/keys"
import type { PanelDataMeta } from "@/dashboards/queries"
import type {
  DashlistPanel,
  DataPanel,
  DataPanelType,
  NewsPanel,
  PlaceholderPanel,
  TextPanel,
} from "@/spec"

import type { FieldResolver, LinkScope } from "./field-config"
import type { PreparedFrame } from "./transforms"

export type PanelRendererProps = {
  panel: DataPanel
  frames: PreparedFrame[]
  fields: FieldResolver
  display: Record<string, unknown>
  meta: PanelDataMeta
  width: number
  height: number
  syncKey: string
  onTimeRangeSelect: (fromMs: number, toMs: number) => void
  links: Omit<
    LinkScope,
    "row" | "seriesName" | "fieldName" | "value" | "valueText"
  >
  ghostFrames?: PreparedFrame[]
}

export type PanelRenderer = ComponentType<PanelRendererProps>

export type StaticPanelType = "text" | "placeholder" | "dashlist" | "news"

export type StaticPanelProps = {
  panel: TextPanel | PlaceholderPanel | DashlistPanel | NewsPanel
  width: number
  height: number
  vars: Selections
  from: string
  to: string
}

export type StaticPanelRenderer = ComponentType<StaticPanelProps>

export type RendererManifest =
  | {
      type: DataPanelType
      supported: boolean
      load: () => Promise<{ default: PanelRenderer }>
    }
  | {
      type: StaticPanelType
      supported: boolean
      load: () => Promise<{ default: StaticPanelRenderer }>
    }
