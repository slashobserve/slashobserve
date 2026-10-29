import {
  dataPanelTypes,
  findPanel,
  validateSpecSchema,
  type Dashboard,
  type Panel,
} from "@/spec"

import type { EditablePanel } from "./draft"

export const missingPanelMessage =
  "This panel is not in the version you are editing."

type IssueScope = "dashboard" | "panel"

export type JsonIssue = {
  path: string
  message: string
  scope: IssueScope
  line?: number
}
export type ParsedPanel =
  { ok: true; panel: EditablePanel } | { ok: false; issues: JsonIssue[] }

export function formatPanel(panel: EditablePanel): string {
  return JSON.stringify(panel, null, 2)
}

export function withPanel(
  spec: Dashboard,
  panelId: string,
  panel: EditablePanel
): Dashboard | null {
  const entry = findPanel(spec, panelId)
  if (!entry) return null
  const copy = structuredClone(spec)
  const [, , top, , child] = entry.pointer.split("/")
  if (child === undefined) {
    copy.panels[Number(top)] = panel
  } else {
    const row = copy.panels[Number(top)]
    if (row.type === "row" && row.panels) row.panels[Number(child)] = panel
  }
  return copy
}

export function jsonErrorLine(
  message: string,
  text: string
): number | undefined {
  const located = /line (\d+) column \d+/.exec(message)
  if (located) return Number(located[1])
  const positioned = /at position (\d+)/.exec(message)
  if (!positioned) return undefined
  const before = text.slice(0, Number(positioned[1]))
  return before.split("\n").length
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value)
}

type PanelKind = "data" | "text" | "dashlist" | "news"

function kindOf(type: unknown): PanelKind | null {
  if (type === "text" || type === "dashlist" || type === "news") return type
  return typeof type === "string" &&
    (dataPanelTypes as readonly string[]).includes(type)
    ? "data"
    : null
}

function typeIssue(
  value: Record<string, unknown>,
  current: Panel
): JsonIssue | null {
  if (value.type === "row" || value.type === "placeholder") {
    return {
      path: "/type",
      message: "Choose a data panel type or text.",
      scope: "panel",
    }
  }
  const next = kindOf(value.type)
  if (next !== null && next !== kindOf(current.type)) {
    return {
      path: "/type",
      message:
        "Use the panel type picker in the editor to change a data panel's type.",
      scope: "panel",
    }
  }
  return null
}

function scopedIssue(
  pointer: string,
  issue: { path: string; message: string }
): JsonIssue {
  if (issue.path === pointer) return { ...issue, path: "", scope: "panel" }
  if (issue.path.startsWith(pointer + "/")) {
    return {
      ...issue,
      path: issue.path.slice(pointer.length),
      scope: "panel",
    }
  }
  return { ...issue, scope: "dashboard" }
}

export function parsePanelJson(
  text: string,
  spec: Dashboard,
  panelId: string
): ParsedPanel {
  let value: unknown
  try {
    value = JSON.parse(text)
  } catch (error) {
    const message = (error as SyntaxError).message
    const line = jsonErrorLine(message, text)
    return {
      ok: false,
      issues: [
        line === undefined
          ? { path: "", message, scope: "panel" }
          : { path: "", message, scope: "panel", line },
      ],
    }
  }
  if (!isRecord(value)) {
    return {
      ok: false,
      issues: [
        {
          path: "",
          message: "The panel must be a JSON object.",
          scope: "panel",
        },
      ],
    }
  }
  if (value.id !== panelId) {
    return {
      ok: false,
      issues: [
        {
          path: "/id",
          message: `The panel id must stay "${panelId}".`,
          scope: "panel",
        },
      ],
    }
  }
  const entry = findPanel(spec, panelId)
  const placed = withPanel(spec, panelId, value as unknown as EditablePanel)
  if (!entry || !placed) {
    return {
      ok: false,
      issues: [{ path: "", message: missingPanelMessage, scope: "dashboard" }],
    }
  }
  const crossing = typeIssue(value, entry.panel)
  if (crossing) return { ok: false, issues: [crossing] }

  const result = validateSpecSchema(placed)
  if (result.valid) {
    return { ok: true, panel: value as unknown as EditablePanel }
  }
  return {
    ok: false,
    issues: result.issues.map((issue) => scopedIssue(entry.pointer, issue)),
  }
}
