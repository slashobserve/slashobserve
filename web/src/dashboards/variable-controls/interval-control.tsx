import { useId } from "react"

import type { IntervalVariable } from "@/spec"

import type { VariableState } from "../use-variables"
import { AUTO, intervalOptions } from "../variables"
import { ValueSelect } from "./value-select"
import { VariableBox, variableLabel } from "./variable-box"

type Props = {
  variable: IntervalVariable
  state: VariableState
  onChange: (values: string[]) => void
}

function optionLabel(value: string): string {
  return value === AUTO ? "Auto" : value
}

export function IntervalControl({ variable, state, onChange }: Props) {
  const id = useId()
  return (
    <VariableBox variable={variable} id={id}>
      <ValueSelect
        id={id}
        label={variableLabel(variable)}
        values={intervalOptions(variable)}
        selected={state.selected[0]}
        optionLabel={optionLabel}
        onChange={(next) => onChange([next])}
      />
    </VariableBox>
  )
}
