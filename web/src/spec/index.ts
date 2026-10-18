import {
  dataPanel,
  placeholderPanel,
  rowPanel,
  textPanel,
  validate,
} from "./validate.gen"
import type { Dashboard } from "./observespec.gen"
import { dataPanelTypes } from "./panels"

export type * from "./observespec.gen"
export * from "./panels"

export type SpecIssue = { path: string; message: string }

export type SpecValidation =
  { valid: true; spec: Dashboard } | { valid: false; issues: SpecIssue[] }

type AjvError = {
  instancePath: string
  keyword: string
  message?: string
  params: Record<string, unknown>
}

type AjvValidate = ((
  value: unknown,
  context?: { instancePath: string }
) => boolean) & {
  errors?: AjvError[] | null
}

const run = validate as unknown as AjvValidate

const panelBranches: Record<string, AjvValidate> = {
  text: textPanel as unknown as AjvValidate,
  placeholder: placeholderPanel as unknown as AjvValidate,
  row: rowPanel as unknown as AjvValidate,
}

function branchFor(panel: Record<string, unknown>): AjvValidate | undefined {
  const type = panel.type
  if (typeof type !== "string") return undefined
  if ((dataPanelTypes as readonly string[]).includes(type)) {
    return dataPanel as unknown as AjvValidate
  }
  return Object.hasOwn(panelBranches, type) ? panelBranches[type] : undefined
}

function toIssue(error: AjvError): SpecIssue {
  if (error.keyword === "required") {
    return {
      path: error.instancePath,
      message: `missing property "${error.params.missingProperty}"`,
    }
  }
  if (error.keyword === "additionalProperties") {
    return {
      path: error.instancePath,
      message: `unknown property "${error.params.additionalProperty}"`,
    }
  }
  return { path: error.instancePath, message: error.message ?? "invalid value" }
}

const aggregateKeywords = new Set(["oneOf", "anyOf", "if"])

function isNoise(error: AjvError): boolean {
  return (
    aggregateKeywords.has(error.keyword) ||
    (error.keyword === "const" && error.instancePath.endsWith("/type"))
  )
}

type PanelSite = { pointer: string; panel: Record<string, unknown> }

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value)
}

function panelSites(value: unknown): PanelSite[] {
  if (!isRecord(value) || !Array.isArray(value.panels)) return []
  const sites: PanelSite[] = []
  value.panels.forEach((panel, i) => {
    if (!isRecord(panel)) return
    sites.push({ pointer: `/panels/${i}`, panel })
    if (panel.type !== "row" || !Array.isArray(panel.panels)) return
    panel.panels.forEach((child, j) => {
      if (isRecord(child)) {
        sites.push({ pointer: `/panels/${i}/panels/${j}`, panel: child })
      }
    })
  })
  return sites
}

function isWithin(path: string, pointer: string): boolean {
  return path === pointer || path.startsWith(pointer + "/")
}

function panelErrors(value: unknown, errors: AjvError[]): AjvError[] {
  const sites = panelSites(value).filter((site) => branchFor(site.panel))
  const kept = errors.filter(
    (error) => !sites.some((site) => isWithin(error.instancePath, site.pointer))
  )
  for (const site of sites) {
    const branch = branchFor(site.panel)!
    branch(site.panel, { instancePath: site.pointer })
    const deeper = sites.filter(
      (other) => other !== site && other.pointer.startsWith(site.pointer + "/")
    )
    for (const error of branch.errors ?? []) {
      if (
        !deeper.some((other) => isWithin(error.instancePath, other.pointer))
      ) {
        kept.push(error)
      }
    }
  }
  return kept
}

const exclusiveMembers = [
  {
    path: /\/overrides\/\d+\/match$/,
    message:
      "match must have exactly one of query, series, seriesRegex, field, fieldType",
  },
  {
    path: /\/mappings\/\d+$/,
    message: "mapping must have exactly one of value, range, regex, special",
  },
]

const shapeKeywords = new Set(["required", "additionalProperties", "oneOf"])

function collapseExclusiveMembers(errors: AjvError[]): {
  errors: AjvError[]
  issues: SpecIssue[]
} {
  const issues: SpecIssue[] = []
  let rest = errors
  for (const error of errors) {
    const rule = exclusiveMembers.find((r) => r.path.test(error.instancePath))
    if (error.keyword !== "oneOf" || !rule) continue
    const path = error.instancePath
    const onlyShape = rest.every(
      (other) =>
        !isWithin(other.instancePath, path) ||
        (other.instancePath === path && shapeKeywords.has(other.keyword))
    )
    if (!onlyShape) continue
    rest = rest.filter((other) => other.instancePath !== path)
    issues.push({ path, message: rule.message })
  }
  return { errors: rest, issues }
}

export function validateSpecSchema(value: unknown): SpecValidation {
  if (run(value)) {
    return { valid: true, spec: value as Dashboard }
  }
  const collapsed = collapseExclusiveMembers(
    panelErrors(value, [...(run.errors ?? [])])
  )
  const seen = new Set<string>()
  const issues = [
    ...collapsed.errors.filter((error) => !isNoise(error)).map(toIssue),
    ...collapsed.issues,
  ].filter((issue) => {
    const key = `${issue.path}\u0000${issue.message}`
    if (seen.has(key)) return false
    seen.add(key)
    return true
  })
  return { valid: false, issues }
}
