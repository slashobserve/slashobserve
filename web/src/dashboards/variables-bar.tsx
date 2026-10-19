import { useSetDashboardSearch } from "./search"
import { VariableSelect } from "./variable-select"
import type { VariableState } from "./use-variables"
import { applyVariableChange } from "./variables"

type Props = { states: VariableState[] }

export function VariablesBar({ states }: Props) {
  const setSearch = useSetDashboardSearch()
  if (states.length === 0) return null
  const variables = states.map((state) => state.variable)
  return (
    <div className="flex flex-wrap items-center gap-2 pr-2.5 pb-2.5 pl-4">
      {states.map((state) => (
        <VariableSelect
          key={state.variable.name}
          state={state}
          onChange={(values) =>
            setSearch((current) => ({
              ...current,
              vars: applyVariableChange(
                variables,
                current.vars,
                state.variable.name,
                values
              ),
            }))
          }
        />
      ))}
    </div>
  )
}
