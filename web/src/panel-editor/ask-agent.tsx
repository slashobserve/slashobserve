import type { Dispatch } from "react"

import type { DraftAction, EditablePanel } from "./draft"

export type AskAgentProps = {
  dashboardId: string
  version: number
  draft: EditablePanel
  dispatch: Dispatch<DraftAction>
  blockedReason: string | null
}

export const AskAgent: (props: AskAgentProps) => null = () => null
