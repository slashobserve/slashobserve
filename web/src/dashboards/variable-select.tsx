import { useId } from "react"
import { RotateCcwIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  Combobox,
  ComboboxChip,
  ComboboxChips,
  ComboboxChipsInput,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxItem,
  ComboboxList,
  ComboboxValue,
  useComboboxAnchor,
} from "@/components/ui/combobox"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"

import { ALL } from "./search"
import type { VariableState } from "./use-variables"
import { normalizeMultiSelection } from "./variables"

type Props = {
  state: VariableState
  onChange: (values: string[]) => void
}

function optionLabel(value: string): string {
  return value === ALL ? "All" : value
}

export function VariableSelect({ state, onChange }: Props) {
  const { variable, status } = state
  const label = variable.label ?? variable.name
  const id = useId()
  const anchor = useComboboxAnchor()
  const values = [
    ...(variable.includeAll ? [ALL] : []),
    ...(state.options ?? []),
  ]

  let control
  if (status === "loading") {
    control = (
      <Button variant="outline" disabled aria-label={label} id={id}>
        Loading...
      </Button>
    )
  } else if (status === "error") {
    control = (
      <>
        <Button variant="outline" disabled aria-label={label} id={id}>
          Failed to load
        </Button>
        <Button
          variant="ghost"
          size="icon"
          aria-label={`Retry loading ${label}`}
          onClick={state.retry}
        >
          <RotateCcwIcon />
        </Button>
      </>
    )
  } else if (variable.multi) {
    control = (
      <Combobox
        multiple
        items={values}
        value={state.selected}
        itemToStringLabel={optionLabel}
        onValueChange={(next) =>
          onChange(
            normalizeMultiSelection(
              state.selected,
              next,
              variable.includeAll ?? false
            )
          )
        }
      >
        <ComboboxChips ref={anchor} className="w-64 max-w-full">
          <ComboboxValue>
            {(selected: string[]) => (
              <>
                {selected.map((value) => (
                  <ComboboxChip key={value}>{optionLabel(value)}</ComboboxChip>
                ))}
                <ComboboxChipsInput id={id} aria-label={label} />
              </>
            )}
          </ComboboxValue>
        </ComboboxChips>
        <ComboboxContent anchor={anchor}>
          <ComboboxEmpty>No values found</ComboboxEmpty>
          <ComboboxList>
            {(item: string) => (
              <ComboboxItem key={item} value={item}>
                {optionLabel(item)}
              </ComboboxItem>
            )}
          </ComboboxList>
        </ComboboxContent>
      </Combobox>
    )
  } else {
    control = (
      <Select
        items={values.map((value) => ({
          value,
          label: optionLabel(value),
        }))}
        value={state.selected[0] ?? null}
        onValueChange={(next) => {
          if (next !== null) onChange([next])
        }}
        disabled={values.length === 0}
      >
        <SelectTrigger id={id} aria-label={label} className="min-w-32">
          <SelectValue placeholder="No values" />
        </SelectTrigger>
        <SelectContent alignItemWithTrigger={false} align="start">
          {values.map((value) => (
            <SelectItem key={value} value={value}>
              {optionLabel(value)}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    )
  }

  return (
    <div className="flex max-w-full min-w-0 items-center gap-2">
      <Label htmlFor={id} className="shrink-0 text-muted-foreground">
        {label}
      </Label>
      {control}
      {state.truncated ? (
        <span className="text-xs text-muted-foreground">
          First 1,000 values
        </span>
      ) : null}
    </div>
  )
}
