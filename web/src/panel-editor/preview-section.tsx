import type { Selections } from "@/api/keys"
import type { Dashboard } from "@/spec"

import type { EditablePanel } from "./draft"

export type PreviewSectionProps = {
  dashboardId: string
  spec: Dashboard
  version: number
  base: EditablePanel
  draft: EditablePanel
  timeRange: { from?: string; to?: string }
  selections: Selections | null
}

export const PreviewSection: (props: PreviewSectionProps) => null = () => null
