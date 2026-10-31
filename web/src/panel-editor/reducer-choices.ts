import { anyReducerIds, reducerIds, reducerLabel } from "@/panels/reducers"
import type { AnyReducer } from "@/spec"

import type { ReducerPool } from "./display-options/types"
import type { Option } from "./field-controls"

export const reducerPools: Record<ReducerPool, readonly AnyReducer[]> = {
  reducer: reducerIds,
  anyReducer: anyReducerIds,
}

export function reducerChoices(
  curated: readonly Option[],
  selected: readonly string[],
  pool: readonly AnyReducer[]
): { shown: Option[]; offered: AnyReducer[] } {
  const shown = [...curated]
  for (const value of selected) {
    if (value === "" || shown.some((option) => option.value === value)) continue
    const known = pool.find((id) => id === value)
    shown.push({
      value,
      label: known === undefined ? value : reducerLabel(known),
    })
  }
  return {
    shown,
    offered: pool.filter((id) => !shown.some((option) => option.value === id)),
  }
}

export function reducerMatches(id: AnyReducer, query: string): boolean {
  const needle = query.trim().toLowerCase()
  return (
    id.toLowerCase().includes(needle) ||
    reducerLabel(id).toLowerCase().includes(needle)
  )
}
