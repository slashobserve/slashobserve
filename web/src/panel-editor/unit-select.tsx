import { useId, useState, type Dispatch } from "react"

import {
  Combobox,
  ComboboxCollection,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxGroup,
  ComboboxInput,
  ComboboxItem,
  ComboboxLabel,
  ComboboxList,
} from "@/components/ui/combobox"
import { Input } from "@/components/ui/input"
import { unitGroups, unitLabel } from "@/lib/unit-catalog"
import { customUnitKinds, parseUnit, type CustomUnitKind } from "@/lib/units"
import type { Unit } from "@/spec"

import type { DraftAction } from "./draft"
import { Field } from "./field-controls"

const customPrefix = "custom:"

type CustomForm = {
  label: string
  input: string
  maxLength: number
  hint: string
}

const customForms: Record<CustomUnitKind, CustomForm> = {
  suffix: {
    label: "Custom suffix",
    input: "Suffix",
    maxLength: 20,
    hint: "Text after the number, for example req",
  },
  prefix: {
    label: "Custom prefix",
    input: "Prefix",
    maxLength: 20,
    hint: "Text before the number, for example $",
  },
  time: {
    label: "Custom date format",
    input: "Date format",
    maxLength: 40,
    hint: "Date format, for example YYYY-MM-DD",
  },
  si: {
    label: "Custom SI unit",
    input: "SI symbol",
    maxLength: 40,
    hint: "SI symbol, for example mF",
  },
  count: {
    label: "Custom count",
    input: "Count text",
    maxLength: 40,
    hint: "Scaled with K, Mil and Bil, then this text, for example req",
  },
  currency: {
    label: "Custom currency",
    input: "Currency symbol",
    maxLength: 40,
    hint: "Symbol before the scaled number, for example €",
  },
}

function customUnit(kind: CustomUnitKind, text: string): Unit | null {
  const value = kind === "prefix" ? text : text.trim()
  return value === "" ? null : `${kind}:${value}`
}

function customText(unit: string | undefined): string {
  const parsed = parseUnit(unit)
  return parsed.kind === "id" ? "" : parsed.text
}

const noneValue = "none"

type UnitChoiceGroup = { label: string; items: string[] }

const catalogChoices: UnitChoiceGroup[] = [
  ...unitGroups.map((group) => ({
    label: group.label,
    items: group.units.map((item) => item.id),
  })),
  {
    label: "Custom",
    items: customUnitKinds.map((kind) => `${customPrefix}${kind}`),
  },
]

function choiceLabel(choice: string): string {
  if (choice.startsWith(customPrefix)) {
    const kind = customUnitKinds.find(
      (candidate) => candidate === choice.slice(customPrefix.length)
    )
    return kind ? customForms[kind].label : choice
  }
  return unitLabel(choice)
}

export function UnitSelect({
  unit,
  dispatch,
  readOnly,
}: {
  unit: string | undefined
  dispatch: Dispatch<DraftAction>
  readOnly: boolean
}) {
  const selectId = useId()
  const textId = useId()
  const hintId = useId()
  const [pending, setPending] = useState<CustomUnitKind | null>(null)
  const [text, setText] = useState(customText(unit))
  const [synced, setSynced] = useState(unit)
  if (unit !== synced) {
    setSynced(unit)
    setText(customText(unit))
  }

  const parsed = parseUnit(unit)
  const id = parsed.kind === "id" ? parsed.id : null
  const plain = id === null || id === "none"
  const kind: CustomUnitKind | null =
    parsed.kind !== "id" ? parsed.kind : plain ? pending : null
  const selected = kind ? `${customPrefix}${kind}` : plain ? noneValue : id
  const listed = unitGroups.some((group) =>
    group.units.some((item) => item.id === selected)
  )
  const choices =
    kind || listed
      ? catalogChoices
      : [{ label: "Current", items: [selected] }, ...catalogChoices]

  const apply = (next: Unit | null) => {
    setSynced(next ?? undefined)
    dispatch({ type: "setUnit", unit: next })
  }

  const choose = (value: string) => {
    if (value.startsWith(customPrefix)) {
      const next =
        customUnitKinds.find(
          (candidate) => candidate === value.slice(customPrefix.length)
        ) ?? "suffix"
      setPending(next)
      apply(customUnit(next, text))
      return
    }
    setPending(null)
    setText("")
    apply(value === noneValue ? null : value)
  }

  return (
    <div className="flex flex-col gap-1.5">
      <Field label="Unit" htmlFor={selectId}>
        <Combobox
          items={choices}
          value={selected}
          disabled={readOnly}
          itemToStringLabel={choiceLabel}
          onValueChange={(next) => {
            if (next !== null) choose(next)
          }}
        >
          <ComboboxInput
            id={selectId}
            disabled={readOnly}
            className="w-full"
            placeholder="Search units"
          />
          <ComboboxContent>
            <ComboboxEmpty>No units found</ComboboxEmpty>
            <ComboboxList>
              {(group: UnitChoiceGroup) => (
                <ComboboxGroup key={group.label} items={group.items}>
                  <ComboboxLabel>{group.label}</ComboboxLabel>
                  <ComboboxCollection>
                    {(item: string) => (
                      <ComboboxItem key={item} value={item}>
                        {choiceLabel(item)}
                      </ComboboxItem>
                    )}
                  </ComboboxCollection>
                </ComboboxGroup>
              )}
            </ComboboxList>
          </ComboboxContent>
        </Combobox>
      </Field>
      {kind ? (
        <>
          <Field label={customForms[kind].input} htmlFor={textId}>
            <Input
              id={textId}
              value={text}
              maxLength={customForms[kind].maxLength}
              readOnly={readOnly}
              aria-describedby={hintId}
              onChange={(event) => {
                setText(event.target.value)
                apply(customUnit(kind, event.target.value))
              }}
            />
          </Field>
          <p id={hintId} className="text-xs text-muted-foreground">
            {customForms[kind].hint}
          </p>
        </>
      ) : null}
    </div>
  )
}
