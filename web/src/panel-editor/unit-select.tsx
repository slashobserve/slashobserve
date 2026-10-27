import { useId, useState, type Dispatch } from "react"

import { Input } from "@/components/ui/input"
import { unitGroups, unitLabel } from "@/lib/unit-catalog"
import { parseUnit } from "@/lib/units"
import type { Unit } from "@/spec"

import type { DraftAction } from "./draft"
import { Field, NativeSelect } from "./field-controls"

type CustomKind = "suffix" | "prefix"

const maxCustomLength = 20
const customPrefix = "custom:"
const customLabels: Record<CustomKind, string> = {
  suffix: "Custom suffix",
  prefix: "Custom prefix",
}
const inputLabels: Record<CustomKind, string> = {
  suffix: "Suffix",
  prefix: "Prefix",
}

function customUnit(kind: CustomKind, text: string): Unit | null {
  const value = kind === "suffix" ? text.trim() : text
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
  const [pending, setPending] = useState<CustomKind | null>(null)
  const [text, setText] = useState(customText(unit))
  const [synced, setSynced] = useState(unit)
  if (unit !== synced) {
    setSynced(unit)
    setText(customText(unit))
  }

  const parsed = parseUnit(unit)
  const id = parsed.kind === "id" ? parsed.id : null
  const plain = id === null || id === "none"
  const kind: CustomKind | null =
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
      const next: CustomKind =
        value.slice(customPrefix.length) === "prefix" ? "prefix" : "suffix"
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
            <option value={`${customPrefix}suffix`}>
              {customLabels.suffix}
            </option>
            <option value={`${customPrefix}prefix`}>
              {customLabels.prefix}
            </option>
          </optgroup>
        </NativeSelect>
      </Field>
      {kind ? (
        <Field label={inputLabels[kind]} htmlFor={textId}>
          <Input
            id={textId}
            value={text}
            maxLength={maxCustomLength}
            readOnly={readOnly}
            onChange={(event) => {
              setText(event.target.value)
              apply(customUnit(kind, event.target.value))
            }}
          />
        </Field>
      ) : null}
    </div>
  )
}
