import type { Dispatch } from "react"

import type { Dashboard } from "@/spec"

import type { DraftAction, EditablePanel } from "./draft"

export type LeaveGuard = (leave: () => void) => void

export type JsonViewProps = {
  spec: Dashboard
  draft: EditablePanel
  dispatch: Dispatch<DraftAction>
  readOnly: boolean
  onValidityChange: (valid: boolean) => void
  onLeaveRequest: (guard: LeaveGuard) => void
}

export const JsonView: (props: JsonViewProps) => null = () => null
