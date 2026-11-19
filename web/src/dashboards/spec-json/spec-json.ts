import { sameJson } from "@/lib/json"
import { jsonErrorLine } from "@/panel-editor/panel-json"
import { dataPanelTypes, type Dashboard, type SpecValidator } from "@/spec"

export type SpecJsonResult =
  | { kind: "unchanged" }
  | { kind: "valid"; spec: Dashboard }
  | { kind: "invalid"; lines: string[] }

const panelKinds = new Set<string>([
  ...dataPanelTypes,
  "text",
  "dashlist",
  "news",
  "row",
  "placeholder",
])

export function formatSpec(spec: Dashboard): string {
  return JSON.stringify(spec, null, 2)
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value)
}

function unknownTypes(value: Record<string, unknown>): Map<string, string> {
  const found = new Map<string, string>()
  const visit = (panels: unknown, prefix: string) => {
    if (!Array.isArray(panels)) return
    panels.forEach((panel: unknown, index) => {
      if (!isRecord(panel)) return
      const pointer = `${prefix}/${index}`
      const type = panel.type
      if (typeof type !== "string") {
        found.set(pointer, "The panel needs a type.")
      } else if (!panelKinds.has(type)) {
        found.set(pointer, `"${type}" is not a panel type.`)
      } else if (type === "row") {
        visit(panel.panels, `${pointer}/panels`)
      }
    })
  }
  visit(value.panels, "/panels")
  return found
}

function within(path: string, pointer: string): boolean {
  return path === pointer || path.startsWith(`${pointer}/`)
}

function specIssueLines(
  value: Record<string, unknown>,
  issues: readonly { path: string; message: string }[]
): string[] {
  const typed = unknownTypes(value)
  const lines = [...typed].map(
    ([pointer, message]) => `${pointer}/type: ${message}`
  )
  for (const issue of issues) {
    if ([...typed.keys()].some((pointer) => within(issue.path, pointer))) {
      continue
    }
    lines.push(`${issue.path || "/"}: ${issue.message}`)
  }
  return [...new Set(lines)]
}

export function evaluateSpecJson(
  text: string,
  saved: Dashboard,
  validate: SpecValidator
): SpecJsonResult {
  let value: unknown
  try {
    value = JSON.parse(text)
  } catch (error) {
    const message = (error as SyntaxError).message
    const line = jsonErrorLine(message, text)
    return {
      kind: "invalid",
      lines: [
        line === undefined || /line \d+/.test(message)
          ? message
          : `Line ${line}: ${message}`,
      ],
    }
  }
  if (!isRecord(value)) {
    return { kind: "invalid", lines: ["The Spec must be a JSON object."] }
  }
  if (sameJson(value, saved)) return { kind: "unchanged" }
  const result = validate(value)
  if (result.valid) return { kind: "valid", spec: result.spec }
  return { kind: "invalid", lines: specIssueLines(value, result.issues) }
}
