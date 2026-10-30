import type { Selections } from "@/api/keys"
import { ALL } from "@/dashboards/search"

export function selectedValues(
  vars: Selections | undefined,
  name: string
): string[] | undefined {
  if (!vars || !Object.hasOwn(vars, name)) return undefined
  const values = vars[name]
  return values.length === 0 || values.includes(ALL) ? undefined : values
}

const reference = /^\$\{([A-Za-z][A-Za-z0-9_]{0,39})\}$/

export function variableOfReference(text: string): string | undefined {
  return reference.exec(text)?.[1]
}
