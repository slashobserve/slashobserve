import type { Selections } from "@/api/keys"
import { ApiError, NetworkError } from "@/api/errors"
import type { paths } from "@/api/schema.gen"
import { actionDefaults } from "@/spec"

import { selectedValues } from "../field-config/variable-format"

export type CanvasActionContext = {
  dashboardId: string
  version: number
  latestVersion: number
  canRun: boolean
}

export type ActionValue = {
  raw: string
  text: string
  numeric: number | null
}

export type ElementAction = {
  index: number
  title: string
  method: string
  host: string
  confirm: string | null
  variables: string[]
  fields: Record<string, string>
}

export type ActionRequestBody =
  paths["/dashboards/{id}/actions"]["post"]["requestBody"]["content"]["application/json"]

export type ActionProblem = { text: string; allowlist: boolean }

export type ActionOutcome = { tone: "success" | "error"; text: string }

export const outsideDashboardReason = "Actions run from the dashboard."
export const needsEditReason =
  "Running actions needs Edit on this dashboard's folder."
export const latestOnlyReason = "Actions run only on the latest version."
export const noActionTitle = "This button has no action yet."
export const sentFromServerLine = "SlashObserve sends this from its server."

const methods = ["GET", "POST", "PUT", "PATCH", "DELETE"]
const hostOfUrl = /^https:\/\/([A-Za-z0-9.-]+)(?::443)?(?:\/|$)/
const identifier = /^[A-Za-z_][A-Za-z0-9_]*/
const builtinNames = new Set([
  "__from",
  "__to",
  "__user.login",
  "__user.email",
  "__user.name",
  "__user.id",
  "__value.raw",
  "__value.text",
  "__value.numeric",
])
const dataFieldsPrefix = "__data.fields."
const maxFields = 50
const maxValueBytes = 1000

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value)
}

export function disabledReason(
  context: CanvasActionContext | null
): string | null {
  if (context === null) return outsideDashboardReason
  if (!context.canRun) return needsEditReason
  if (context.version !== context.latestVersion) return latestOnlyReason
  return null
}

export function actionHost(url: unknown): string | null {
  if (typeof url !== "string") return null
  const match = hostOfUrl.exec(url)
  return match ? match[1].toLowerCase() : null
}

function referenceNames(template: string): string[] {
  const names: string[] = []
  for (let at = 0; at < template.length; at++) {
    if (template[at] !== "$") continue
    const rest = template.slice(at + 1)
    if (rest.startsWith("{")) {
      const closing = rest.indexOf("}")
      if (closing < 0) break
      const [name] = rest.slice(1, closing).split(":", 1)
      if (name !== "") names.push(name)
      at += closing + 1
      continue
    }
    const ident = identifier.exec(rest)
    if (ident) {
      names.push(ident[0])
      at += ident[0].length
    }
  }
  return names
}

function templatesOf(raw: Record<string, unknown>): string[] {
  const values = (params: unknown): string[] =>
    Array.isArray(params)
      ? params.flatMap((param) =>
          isRecord(param) && typeof param.value === "string"
            ? [param.value]
            : []
        )
      : []
  return [
    typeof raw.url === "string" ? raw.url : "",
    typeof raw.body === "string" ? raw.body : "",
    ...values(raw.query),
    ...values(raw.headers),
  ]
}

function byteLength(text: string): number {
  return new TextEncoder().encode(text).length
}

export function parseActions(
  raw: unknown,
  fieldValue: (name: string) => string | null
): ElementAction[] {
  if (!Array.isArray(raw)) return []
  return raw.flatMap((entry, index): ElementAction[] => {
    if (!isRecord(entry)) return []
    const host = actionHost(entry.url)
    if (
      host === null ||
      typeof entry.title !== "string" ||
      entry.title === ""
    ) {
      return []
    }
    const names = [...new Set(templatesOf(entry).flatMap(referenceNames))]
    const fields: Record<string, string> = {}
    const variables: string[] = []
    for (const name of names) {
      if (builtinNames.has(name)) continue
      if (!name.startsWith(dataFieldsPrefix)) {
        variables.push(name)
        continue
      }
      const field = name.slice(dataFieldsPrefix.length)
      const value = fieldValue(field)
      if (
        value !== null &&
        byteLength(value) <= maxValueBytes &&
        Object.keys(fields).length < maxFields
      ) {
        fields[field] = value
      }
    }
    return [
      {
        index,
        title: entry.title,
        method:
          typeof entry.method === "string" && methods.includes(entry.method)
            ? entry.method
            : actionDefaults.method,
        host,
        confirm:
          typeof entry.confirm === "string" && entry.confirm !== ""
            ? entry.confirm
            : null,
        variables,
        fields,
      },
    ]
  })
}

export function actionRequest({
  context,
  panelId,
  elementId,
  value,
  action,
  vars,
  fromMs,
  toMs,
}: {
  context: CanvasActionContext
  panelId: string
  elementId: string
  value: ActionValue | null
  action: ElementAction
  vars: Selections
  fromMs: number
  toMs: number
}): ActionRequestBody {
  const variables: Record<string, string[]> = {}
  for (const name of action.variables) {
    const selected = selectedValues(vars, name)
    if (selected !== null) variables[name] = selected
  }
  return {
    version: context.latestVersion,
    panel: panelId,
    element: elementId,
    action: action.index,
    variables,
    from: new Date(fromMs).toISOString(),
    to: new Date(toMs).toISOString(),
    value,
    fields: action.fields,
  }
}

export function requestLine(action: ElementAction): string {
  return `${action.method} to ${action.host}`
}

export function actionOutcome(
  title: string,
  result: { status: number; ok: boolean }
): ActionOutcome {
  return result.ok
    ? { tone: "success", text: `Ran ${title}.` }
    : { tone: "error", text: `${title} answered ${result.status}.` }
}

export function actionProblem(error: unknown, host: string): ActionProblem {
  const plain = (text: string): ActionProblem => ({ text, allowlist: false })
  if (error instanceof NetworkError) {
    return plain("Could not reach SlashObserve. Check your connection.")
  }
  if (!(error instanceof ApiError)) return plain("Could not run this action.")
  switch (error.code) {
    case "outbound_domain_not_allowed":
      return { text: `${host} is not allowed on this server.`, allowlist: true }
    case "stale_version":
      return plain("The dashboard changed. Reload it to run this action.")
    case "too_many_actions":
      return plain("Wait for your other actions to finish.")
    case "action_failed":
      return plain(`${host} did not answer.`)
    case "action_not_found":
      return plain("This action is no longer in the dashboard.")
    case "action_invalid":
      return plain(
        "This action cannot be built from the dashboard's current values."
      )
    case "forbidden_role":
    case "forbidden":
      return plain(needsEditReason)
    default:
      return plain("Could not run this action.")
  }
}
