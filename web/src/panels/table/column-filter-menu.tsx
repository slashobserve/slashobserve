import { useMemo } from "react"
import { FilterIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import {
  Popover,
  PopoverContent,
  PopoverTitle,
  PopoverTrigger,
} from "@/components/ui/popover"

import { distinctValues, maxFilterValues } from "./column-filter"
import type { TableRow } from "./table-model"

type MenuProps = {
  label: string
  rows: TableRow[]
  textOf: (row: TableRow) => string
  kept: ReadonlySet<string>
  onToggle: (value: string) => void
  onClear: () => void
}

function ValueList({
  label,
  rows,
  textOf,
  kept,
  onToggle,
}: Omit<MenuProps, "onClear">) {
  const { values, total } = useMemo(
    () => distinctValues(rows, textOf),
    [rows, textOf]
  )
  return (
    <>
      <ul className="flex flex-col gap-1" aria-label={`Values of ${label}`}>
        {values.map((value) => (
          <li key={value}>
            <label className="flex items-center gap-2 break-all">
              <Checkbox
                checked={kept.has(value)}
                onCheckedChange={() => onToggle(value)}
              />
              <span>{value === "" ? "(empty)" : value}</span>
            </label>
          </li>
        ))}
      </ul>
      {total > maxFilterValues ? (
        <p className="text-muted-foreground">
          Showing {maxFilterValues} of {total.toLocaleString("en")} values
        </p>
      ) : null}
    </>
  )
}

export function ColumnFilterMenu({ onClear, ...list }: MenuProps) {
  const { label, kept } = list
  const active = kept.size > 0
  return (
    <Popover>
      <PopoverTrigger
        render={
          <Button
            variant="ghost"
            size="icon-xs"
            className={active ? "text-foreground" : "text-muted-foreground"}
            aria-label={
              active
                ? `Filter ${label} (${kept.size} selected)`
                : `Filter ${label}`
            }
          />
        }
      >
        <FilterIcon
          className={active ? "fill-current" : undefined}
          aria-hidden="true"
        />
      </PopoverTrigger>
      <PopoverContent align="start" className="max-h-72 w-60 overflow-auto">
        <PopoverTitle>Filter {label}</PopoverTitle>
        <ValueList {...list} />
        <Button
          variant="outline"
          size="sm"
          onClick={onClear}
          disabled={!active}
        >
          Clear filter
        </Button>
      </PopoverContent>
    </Popover>
  )
}
