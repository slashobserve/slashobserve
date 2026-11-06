import type { SpecValidator } from "./validate"

export type * from "./observespec.gen"
export * from "./datasource-ref"
export * from "./defaults"
export { displayKeysByType, type DisplayKey } from "./display-keys.gen"
export * from "./panels"
export * from "./patterns"
export type { SpecIssue, SpecValidation, SpecValidator } from "./validate"

export function loadSpecValidator(): Promise<SpecValidator> {
  return import("./validate").then((module) => module.validateSpecSchema)
}
