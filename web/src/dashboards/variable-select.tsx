import { useId } from "react"
import { Combobox as ComboboxPrimitive } from "@base-ui/react"
import { ChevronDownIcon, LoaderCircleIcon, RotateCcwIcon } from "lucide-react"

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

const controlButton = "h-auto rounded-none px-[9px] text-[13px]"

function optionLabel(value: string): string {
  return value === ALL ? "All" : value
}

export function VariableSelect({ state, onChange }: Props) {
  const { variable, status } = state
  const refreshing = status === "refreshing"
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
      <Button
        variant="ghost"
        disabled
        aria-label={`${label}, loading`}
        id={id}
        className={controlButton}
      >
        Loading...
      </Button>
    )
  } else if (status === "error") {
    control = (
      <>
        <Button
          variant="ghost"
          disabled
          aria-label={`${label}, failed to load`}
          id={id}
          className={controlButton}
        >
          Failed to load
        </Button>
        <Button
          variant="ghost"
          size="icon-sm"
          className="rounded-none"
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
        disabled={refreshing}
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
        <ComboboxChips
          ref={anchor}
          className="min-h-[26px] max-w-full min-w-0 flex-wrap gap-1.5 rounded-none border-0 py-0.5 pr-[9px] pl-[9px] focus-within:border-0 focus-within:ring-0 has-data-[slot=combobox-chip]:px-[9px]"
          aria-busy={refreshing || undefined}
        >
          <ComboboxValue>
            {(selected: string[]) => (
              <>
                {selected.map((value) => (
                  <ComboboxChip key={value}>{optionLabel(value)}</ComboboxChip>
                ))}
                <ComboboxChipsInput
                  id={id}
                  className="field-sizing-content w-auto min-w-1 flex-none narrow:leading-4"
                  aria-label={label}
                  placeholder={
                    refreshing && state.selected.length === 0
                      ? "Loading..."
                      : undefined
                  }
                  aria-busy={refreshing || undefined}
                />
              </>
            )}
          </ComboboxValue>
          {refreshing ? (
            <LoaderCircleIcon
              aria-hidden="true"
              className="pointer-events-none ml-auto size-3 shrink-0 self-center text-muted-foreground motion-safe:animate-spin"
            />
          ) : (
            <ComboboxPrimitive.Trigger
              aria-label={`Show ${label} values`}
              aria-hidden="true"
              tabIndex={-1}
              className="ml-auto grid shrink-0 place-items-center self-center outline-none"
            >
              <ChevronDownIcon
                aria-hidden="true"
                className="pointer-events-none size-3 text-muted-foreground"
              />
            </ComboboxPrimitive.Trigger>
          )}
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
        disabled={refreshing || values.length === 0}
      >
        <SelectTrigger
          id={id}
          aria-label={label}
          aria-busy={refreshing || undefined}
          className="min-w-24 gap-1.5 rounded-none border-0 px-[9px] py-0 focus-visible:ring-0 data-[size=default]:h-auto"
          icon={
            refreshing ? (
              <LoaderCircleIcon
                aria-hidden="true"
                className="pointer-events-none size-3 text-muted-foreground motion-safe:animate-spin"
              />
            ) : (
              <ChevronDownIcon
                aria-hidden="true"
                className="pointer-events-none size-3 text-muted-foreground"
              />
            )
          }
        >
          <SelectValue placeholder={refreshing ? "Loading..." : "No values"} />
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
      <div className="flex min-h-7 max-w-full min-w-0 items-stretch rounded-control border text-[13px] has-focus-visible:ring-2 has-focus-visible:ring-ring">
        <Label
          htmlFor={id}
          className="flex shrink-0 items-center border-r px-[9px] text-[13px] font-normal text-muted-foreground"
        >
          {label}
        </Label>
        {control}
      </div>
      {state.truncated ? (
        <span className="text-xs text-muted-foreground">
          First 1,000 values
        </span>
      ) : null}
    </div>
  )
}
