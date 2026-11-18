import type { VariableState } from "../use-variables"

type Header = {
  folder: string | null
  version: number
  range: string
  variables: string[]
}

export function variableLines(states: VariableState[]): string[] {
  return states
    .filter((state) => state.selected.length > 0)
    .map(
      (state) =>
        `${state.variable.label ?? state.variable.name}: ${state.selected.join(", ")}`
    )
}

export function headerLines({ folder, version, range, variables }: Header) {
  return {
    context: [folder, `version ${version}`]
      .filter((part) => part !== null)
      .join(", "),
    range: `Time range: ${range}`,
    variables: variables.length > 0 ? variables.join("; ") : null,
  }
}

export function printedAt(at: Date, locale?: string, timeZone?: string) {
  return `Printed ${at.toLocaleString(locale, {
    dateStyle: "medium",
    timeStyle: "short",
    timeZone,
  })}`
}
