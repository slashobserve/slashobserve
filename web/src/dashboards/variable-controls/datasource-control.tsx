import { useId } from "react"

import type { DatasourceVariable } from "@/spec"

import type { VariableState } from "../use-variables"
import { FailedControl, LoadingControl, ValueSelect } from "./value-select"
import { VariableBox, variableLabel } from "./variable-box"

type Props = {
  variable: DatasourceVariable
  state: VariableState
  onChange: (values: string[]) => void
}

export function DatasourceControl({ variable, state, onChange }: Props) {
  const id = useId()
  const label = variableLabel(variable)
  const { status } = state

  let control
  if (status === "loading") {
    control = <LoadingControl id={id} label={label} />
  } else if (status === "error") {
    control = <FailedControl id={id} label={label} retry={state.retry} />
  } else {
    control = (
      <ValueSelect
        id={id}
        label={label}
        values={state.options ?? []}
        selected={state.selected[0]}
        refreshing={status === "refreshing"}
        onChange={(next) => onChange([next])}
      />
    )
  }

  return (
    <VariableBox variable={variable} id={id} truncated={state.truncated}>
      {control}
    </VariableBox>
  )
}
