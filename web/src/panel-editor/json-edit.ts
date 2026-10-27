import type { Dashboard } from "@/spec"
import { sameJson, stableJson } from "@/lib/json"

import type { EditablePanel } from "./draft"
import { formatPanel, parsePanelJson, type JsonIssue } from "./panel-json"

export const maxShownIssues = 20

export type JsonEdit =
  | { kind: "valid"; panel: EditablePanel; changesDraft: boolean }
  | { kind: "invalid"; issues: JsonIssue[] }

export type IssueRow = {
  location: { kind: "line" | "path"; text: string } | null
  message: string
}

export function evaluateJson(
  text: string,
  spec: Dashboard,
  draft: EditablePanel
): JsonEdit {
  const parsed = parsePanelJson(text, spec, draft.id)
  if (!parsed.ok) return { kind: "invalid", issues: parsed.issues }
  return {
    kind: "valid",
    panel: parsed.panel,
    changesDraft: !sameJson(parsed.panel, draft),
  }
}

export function appliedKey(panel: EditablePanel): string {
  return stableJson(panel)
}

export function textForDraft(
  draft: EditablePanel,
  applied: string,
  textIsValid: boolean
): string | null {
  if (!textIsValid || appliedKey(draft) === applied) return null
  return formatPanel(draft)
}

function issueRow(issue: JsonIssue): IssueRow {
  if (issue.line !== undefined) {
    return {
      location: { kind: "line", text: `Line ${issue.line}` },
      message: issue.message,
    }
  }
  if (issue.path !== "") {
    return {
      location: { kind: "path", text: issue.path },
      message: issue.message,
    }
  }
  return { location: null, message: issue.message }
}

export function issueRows(issues: readonly JsonIssue[]): {
  rows: IssueRow[]
  more: number
} {
  return {
    rows: issues.slice(0, maxShownIssues).map(issueRow),
    more: Math.max(0, issues.length - maxShownIssues),
  }
}
