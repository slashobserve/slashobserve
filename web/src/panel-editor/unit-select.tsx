import { useId, useState, type Dispatch } from "react"

import { Input } from "@/components/ui/input"
import { unitGroups, unitLabel } from "@/lib/unit-catalog"
import { customUnitKinds, parseUnit, type CustomUnitKind } from "@/lib/units"
import type { Unit } from "@/spec"

import type { DraftAction } from "./draft"
import { Field, NativeSelect } from "./field-controls"

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
  const selected = kind ? `${customPrefix}${kind}` : plain ? "" : id
  const listed = unitGroups.some((group) =>
    group.units.some((item) => item.id === selected)
  )

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
    apply(value === "" ? null : value)
  }

  return (
    <div className="flex flex-col gap-1.5">
      <Field label="Unit" htmlFor={selectId}>
        <NativeSelect
          id={selectId}
          value={selected}
          disabled={readOnly}
          onChange={(event) => choose(event.target.value)}
        >
          <option value="">None</option>
          {selected === "" || kind || listed ? null : (
            <option value={selected}>{unitLabel(unit)}</option>
          )}
          {unitGroups.map((group) => (
            <optgroup key={group.label} label={group.label}>
              {group.units
                .filter((item) => item.id !== "none")
                .map((item) => (
                  <option key={item.id} value={item.id}>
                    {item.label}
                  </option>
                ))}
            </optgroup>
          ))}
          <optgroup label="Custom">
            {customUnitKinds.map((custom) => (
              <option key={custom} value={`${customPrefix}${custom}`}>
                {customForms[custom].label}
              </option>
            ))}
          </optgroup>
        </NativeSelect>
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
