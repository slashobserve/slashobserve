import type { DatasourceVariable, Variable } from "./observespec.gen"

const referencePattern = /^\$\{([A-Za-z][A-Za-z0-9_]{0,39})\}$/

const allSelection = "$__all"

export function datasourceVariableName(ref: string): string | undefined {
  return referencePattern.exec(ref)?.[1]
}

export function resolveDatasourceRef(
  ref: string,
  selections: Record<string, string[]>
): string | undefined {
  const name = datasourceVariableName(ref)
  if (name === undefined) return ref
  const selected = Object.hasOwn(selections, name) ? selections[name] : []
  return selected.length === 1 && selected[0] !== allSelection
    ? selected[0]
    : undefined
}

export function datasourceRefType(
  ref: string,
  variables: readonly Variable[],
  typeOfUid: (uid: string) => string | undefined
): string | undefined {
  const name = datasourceVariableName(ref)
  if (name === undefined) return typeOfUid(ref)
  return variables.find(
    (variable): variable is DatasourceVariable =>
      variable.type === "datasource" && variable.name === name
  )?.datasourceType
}

export function datasourceVariableDefaults(
  variables: readonly Variable[]
): Record<string, string[]> {
  const defaults: Record<string, string[]> = {}
  for (const variable of variables) {
    if (variable.type === "datasource" && variable.default) {
      defaults[variable.name] = [variable.default]
    }
  }
  return defaults
}
