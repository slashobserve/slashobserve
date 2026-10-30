import { Fragment } from "react"

import { datasourceTypeOf, useDatasources } from "@/datasources/queries"
import { datasourceRefType, resolveDatasourceRef } from "@/spec"

import { useSetDashboardSearch } from "./search"
import { UnverifiedLabel } from "./unverified-label"
import type { VariableState } from "./use-variables"
import { AdhocControl } from "./variable-controls/adhoc-control"
import { DatasourceControl } from "./variable-controls/datasource-control"
import { GroupByControl } from "./variable-controls/groupby-control"
import { IntervalControl } from "./variable-controls/interval-control"
import { TextboxControl } from "./variable-controls/textbox-control"
import { variableLabel } from "./variable-controls/variable-box"
import { VariableSelect } from "./variable-select"
import { applyVariableChange } from "./variables"

type Props = { states: VariableState[]; unverified: Map<string, string> }

function isShown(state: VariableState): boolean {
  return (
    state.variable.type !== "constant" && state.variable.hide !== "variable"
  )
}

export function VariablesBar({ states, unverified }: Props) {
  const setSearch = useSetDashboardSearch()
  const datasources = useDatasources()
  const shown = states.filter(isShown)
  if (shown.length === 0) return null
  const variables = states.map((state) => state.variable)
  const selections = Object.fromEntries(
    states.map((state) => [state.variable.name, state.selected])
  )

  function control(state: VariableState) {
    const { variable } = state
    const onChange = (values: string[]) =>
      setSearch((current) => ({
        ...current,
        vars: applyVariableChange(
          variables,
          current.vars,
          variable.name,
          values
        ),
      }))
    switch (variable.type) {
      case "query":
      case "custom":
        return (
          <VariableSelect
            variable={variable}
            state={state}
            onChange={onChange}
          />
        )
      case "interval":
        return (
          <IntervalControl
            variable={variable}
            state={state}
            onChange={onChange}
          />
        )
      case "textbox":
        return (
          <TextboxControl
            variable={variable}
            state={state}
            onChange={onChange}
          />
        )
      case "datasource":
        return (
          <DatasourceControl
            variable={variable}
            state={state}
            onChange={onChange}
          />
        )
      case "adhoc":
        return (
          <AdhocControl
            variable={variable}
            state={state}
            datasource={resolveDatasourceRef(variable.datasource, selections)}
            datasourceType={datasourceRefType(
              variable.datasource,
              variables,
              (uid) => datasourceTypeOf(datasources.data, uid)
            )}
            onChange={onChange}
          />
        )
      case "groupby":
        return (
          <GroupByControl
            variable={variable}
            state={state}
            datasource={resolveDatasourceRef(variable.datasource, selections)}
            datasourceType={datasourceRefType(
              variable.datasource,
              variables,
              (uid) => datasourceTypeOf(datasources.data, uid)
            )}
            onChange={onChange}
          />
        )
      case "constant":
        return null
    }
  }

  return (
    <div className="flex flex-wrap items-center gap-2 pr-2.5 pb-2.5 pl-4">
      {shown.map((state) => {
        const { variable } = state
        const reason = unverified.get(variable.name)
        return (
          <Fragment key={variable.name}>
            {control(state)}
            {reason ? (
              <UnverifiedLabel
                title={variableLabel(variable)}
                reason={reason}
              />
            ) : null}
          </Fragment>
        )
      })}
    </div>
  )
}
