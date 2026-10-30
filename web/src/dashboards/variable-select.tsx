import { useId } from "react"
import { Combobox as ComboboxPrimitive } from "@base-ui/react"
import { ChevronDownIcon, LoaderCircleIcon } from "lucide-react"

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
import type { CustomVariable, QueryVariable } from "@/spec"

import { ALL } from "./search"
import type { VariableState } from "./use-variables"
import {
  FailedControl,
  LoadingControl,
  ValueSelect,
} from "./variable-controls/value-select"
import { variableLabel, VariableBox } from "./variable-controls/variable-box"
import { normalizeMultiSelection, optionTexts } from "./variables"

type Props = {
  variable: QueryVariable | CustomVariable
  state: VariableState
  onChange: (values: string[]) => void
}

export function VariableSelect({ variable, state, onChange }: Props) {
  const { status } = state
  const refreshing = status === "refreshing"
  const label = variableLabel(variable)
  const id = useId()
  const anchor = useComboboxAnchor()
  const values = [
    ...(variable.includeAll ? [ALL] : []),
    ...(state.options ?? []),
  ]
  const textOf = optionTexts({
    values: state.options ?? [],
    texts: state.texts,
  })
  const optionLabel = (value: string) => (value === ALL ? "All" : textOf(value))

  let control
  if (status === "loading") {
    control = <LoadingControl id={id} label={label} />
  } else if (status === "error") {
    control = <FailedControl id={id} label={label} retry={state.retry} />
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
          className="min-h-[26px] max-w-full min-w-0 flex-nowrap gap-1.5 rounded-none border-0 py-0.5 pr-[9px] pl-[9px] focus-within:border-0 focus-within:ring-0 has-data-[slot=combobox-chip]:px-[9px]"
          aria-busy={refreshing || undefined}
        >
          <div className="flex min-w-0 flex-1 flex-wrap items-center gap-1.5">
            <ComboboxValue>
              {(selected: string[]) => (
                <>
                  {selected.map((value) => (
                    <ComboboxChip key={value}>
                      {optionLabel(value)}
                    </ComboboxChip>
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
          </div>
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
      <ValueSelect
        id={id}
        label={label}
        values={values}
        selected={state.selected[0]}
        refreshing={refreshing}
        optionLabel={optionLabel}
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
