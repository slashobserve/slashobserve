import type { ReactNode } from "react"

import { Label } from "@/components/ui/label"
import type { Variable } from "@/spec"

export function variableLabel(variable: Variable): string {
  return variable.label ?? variable.name
}

type Props = {
  variable: Variable
  id: string
  truncated?: boolean
  children: ReactNode
}

export function VariableBox({
  variable,
  id,
  truncated = false,
  children,
}: Props) {
  return (
    <div className="flex max-w-full min-w-0 items-center gap-2">
      <div className="flex min-h-7 max-w-full min-w-0 items-stretch rounded-control border text-[13px] has-focus-visible:ring-2 has-focus-visible:ring-ring">
        {variable.hide === "label" ? null : (
          <Label
            htmlFor={id}
            title={variable.description}
            className="flex shrink-0 items-center border-r px-[9px] text-[13px] font-normal text-muted-foreground"
          >
            {variableLabel(variable)}
          </Label>
        )}
        {children}
      </div>
      {truncated ? (
        <span className="text-xs text-muted-foreground">
          First 1,000 values
        </span>
      ) : null}
    </div>
  )
}
