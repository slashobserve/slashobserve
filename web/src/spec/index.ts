import { validate } from "./validate.gen"
import type { Dashboard } from "./observespec.gen"

export type * from "./observespec.gen"

export type SpecIssue = { path: string; message: string }

export type SpecValidation =
  { valid: true; spec: Dashboard } | { valid: false; issues: SpecIssue[] }

type AjvError = {
  instancePath: string
  keyword: string
  message?: string
  params: Record<string, unknown>
}

type AjvValidate = ((value: unknown) => boolean) & {
  errors?: AjvError[] | null
}

const run = validate as unknown as AjvValidate

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

export function validateSpec(value: unknown): SpecValidation {
  if (run(value)) {
    return { valid: true, spec: value as Dashboard }
  }
  return { valid: false, issues: (run.errors ?? []).map(toIssue) }
}
