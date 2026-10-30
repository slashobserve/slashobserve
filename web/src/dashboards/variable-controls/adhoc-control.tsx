import { useId, useState } from "react"
import { PlusIcon, XIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"
import type { AdhocVariable } from "@/spec"

import type { VariableState } from "../use-variables"
import {
  describeFilter,
  formatFilter,
  parseFilter,
  type AdhocFilter,
} from "./adhoc"
import { AdhocFilterForm } from "./adhoc-filter"
import { VariableBox, variableLabel } from "./variable-box"

type Props = {
  variable: AdhocVariable
  state: VariableState
  datasource: string | undefined
  datasourceType: string | undefined
  onChange: (values: string[]) => void
}

export function AdhocControl({
  variable,
  state,
  datasource,
  datasourceType,
  onChange,
}: Props) {
  const id = useId()
  const [open, setOpen] = useState(false)
  const filters = state.selected.flatMap((text) => {
    const filter = parseFilter(text)
    return filter ? [{ text, filter }] : []
  })

  function add(filter: AdhocFilter) {
    const text = formatFilter(filter)
    const current = filters.map((item) => item.text)
    if (!current.includes(text)) onChange([...current, text])
    setOpen(false)
  }

  return (
    <VariableBox variable={variable} id={id}>
      <div
        role="group"
        aria-label={variableLabel(variable)}
        className="flex min-w-0 flex-wrap items-center gap-1 px-1 py-0.5"
      >
        {filters.map(({ text, filter }) => (
          <span
            key={text}
            className="flex h-5 items-center gap-0.5 rounded-sm bg-muted pl-1.5 text-xs"
          >
            <span className="font-mono">{describeFilter(filter)}</span>
            <Button
              variant="ghost"
              size="icon-xs"
              className="size-5 opacity-60 hover:opacity-100"
              aria-label={`Remove filter ${describeFilter(filter)}`}
              onClick={() =>
                onChange(state.selected.filter((item) => item !== text))
              }
            >
              <XIcon />
            </Button>
          </span>
        ))}
        <Popover open={open} onOpenChange={setOpen}>
          <PopoverTrigger
            render={
              <Button
                variant="ghost"
                size="sm"
                className="h-5 rounded-control px-1.5 text-[13px] font-normal"
              />
            }
          >
            <PlusIcon />
            Add filter
          </PopoverTrigger>
          <PopoverContent align="start" className="w-72">
            <AdhocFilterForm
              datasource={datasource ?? variable.datasource}
              datasourceType={datasourceType}
              count={filters.length}
              onAdd={add}
            />
          </PopoverContent>
        </Popover>
      </div>
    </VariableBox>
  )
}
