import type { Ref } from "react"

import {
  Combobox,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxInput,
  ComboboxItem,
  ComboboxList,
} from "@/components/ui/combobox"
import { Input } from "@/components/ui/input"

const typedReasons = new Set<string>(["input-change", "input-paste"])

type Props = {
  value: string
  onChange: (next: string) => void
  suggestions: readonly string[] | null
  inputRef: Ref<HTMLInputElement>
  placeholder: string
  invalid: boolean
  describedBy: string | undefined
}

export function GranteeInput({
  value,
  onChange,
  suggestions,
  inputRef,
  placeholder,
  invalid,
  describedBy,
}: Props) {
  if (suggestions === null) {
    return (
      <Input
        ref={inputRef}
        aria-label="Add a person or group"
        placeholder={placeholder}
        autoComplete="off"
        spellCheck={false}
        value={value}
        aria-invalid={invalid}
        aria-describedby={describedBy}
        onChange={(event) => onChange(event.target.value)}
      />
    )
  }
  return (
    <Combobox<string>
      items={suggestions}
      value={null}
      inputValue={value}
      onInputValueChange={(next, details) => {
        if (typedReasons.has(details.reason)) onChange(next)
      }}
      onValueChange={(item) => {
        if (item !== null) onChange(item)
      }}
    >
      <ComboboxInput
        ref={inputRef}
        aria-label="Add a person or group"
        placeholder={placeholder}
        showTrigger={false}
        autoComplete="off"
        spellCheck={false}
        aria-invalid={invalid}
        aria-describedby={describedBy}
        className="w-full"
      />
      <ComboboxContent>
        <ComboboxEmpty>
          No signed-in person matches. A group is added as typed.
        </ComboboxEmpty>
        <ComboboxList>
          {(item: string) => (
            <ComboboxItem key={item} value={item}>
              {item}
            </ComboboxItem>
          )}
        </ComboboxList>
      </ComboboxContent>
    </Combobox>
  )
}
