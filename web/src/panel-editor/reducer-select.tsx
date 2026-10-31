import { useId, useState } from "react"

import {
  Combobox,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxInput,
  ComboboxItem,
  ComboboxList,
} from "@/components/ui/combobox"
import { reducerLabel } from "@/panels/reducers"
import type { AnyReducer } from "@/spec"

import { Field } from "./field-controls"
import { reducerMatches } from "./reducer-choices"

export function ReducerSelect({
  label,
  offered,
  disabled,
  onPick,
}: {
  label: string
  offered: readonly AnyReducer[]
  disabled: boolean
  onPick: (reducer: AnyReducer) => void
}) {
  const id = useId()
  const [query, setQuery] = useState("")
  return (
    <Field label={label} htmlFor={id}>
      <Combobox<AnyReducer>
        items={offered}
        value={null}
        inputValue={query}
        disabled={disabled}
        itemToStringLabel={reducerLabel}
        filter={(item, text) => reducerMatches(item, text)}
        onInputValueChange={(text, details) =>
          setQuery(details.reason === "item-press" ? "" : text)
        }
        onValueChange={(next) => {
          if (next !== null) onPick(next)
        }}
      >
        <ComboboxInput
          id={id}
          disabled={disabled}
          placeholder="Search by name or id"
        />
        <ComboboxContent>
          <ComboboxEmpty>No reducers found</ComboboxEmpty>
          <ComboboxList>
            {(item: AnyReducer) => (
              <ComboboxItem key={item} value={item}>
                <span>{reducerLabel(item)}</span>
                <span className="ml-auto font-mono text-xs text-muted-foreground">
                  {item}
                </span>
              </ComboboxItem>
            )}
          </ComboboxList>
        </ComboboxContent>
      </Combobox>
    </Field>
  )
}
