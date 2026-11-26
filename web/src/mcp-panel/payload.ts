import type { PanelDataResponse } from "@/dashboards/queries"
import type { DatasourceRef } from "@/datasources/queries"
import { isDataPanel, type DataPanel, type Panel } from "@/spec"

export type PanelPayload = {
  title: string
  panel: DataPanel
  data: PanelDataResponse
  datasources: DatasourceRef[]
  variables: Record<string, string[]>
  timezone?: string
  from: string
  to: string
  open_url: string
}

export type ToolResult = {
  content?: { type: string; text?: string }[]
  structuredContent?: unknown
  isError?: boolean
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value)
}

export function payloadOf(result: ToolResult): PanelPayload | undefined {
  const raw = result.structuredContent
  if (result.isError || !isRecord(raw)) return undefined
  const { panel, data } = raw
  if (!isRecord(panel) || !isDataPanel(panel as unknown as Panel))
    return undefined
  if (!isRecord(data) || !Array.isArray(data.frames) || !isRecord(data.meta)) {
    return undefined
  }
  return raw as unknown as PanelPayload
}

export function failureOf(result: ToolResult): string {
  const text = result.content?.find((part) => part.type === "text")?.text
  return text ?? "The panel could not be shown."
}
