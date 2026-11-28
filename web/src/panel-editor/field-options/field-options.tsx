import type { Dispatch } from "react"
import { ChevronRightIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import type { DataPanel } from "@/spec"

import { EntryField, useEntry } from "../display-options"
import type { DraftAction } from "../draft"
import { CheckboxField, SelectField } from "../field-controls"
import {
  formSchema,
  optionGroupsFor,
  optionLabels,
  valueLabel,
  type FormKey,
  type OptionGroup,
} from "./groups"
import {
  entryHint,
  entryPlaceholder,
  enumDefaultLabel,
  optionDefault,
  parseOptionEntry,
} from "./values"

type OptionProps = {
  name: FormKey
  stored: unknown
  set: (value: unknown) => void
  disabled: boolean
}

const triState = [
  { value: "", label: "Default" },
  { value: "true", label: "On" },
  { value: "false", label: "Off" },
]

function ToggleOption({ name, stored, set, disabled }: OptionProps) {
  const fallback = optionDefault(name)
  if (typeof fallback !== "boolean") {
    return (
      <SelectField
        label={optionLabels[name]}
        value={typeof stored === "boolean" ? String(stored) : ""}
        options={triState}
        disabled={disabled}
        onChange={(value) => set(value === "" ? undefined : value === "true")}
      />
    )
  }
  return (
    <CheckboxField
      label={optionLabels[name]}
      checked={typeof stored === "boolean" ? stored : fallback}
      disabled={disabled}
      onChange={set}
    />
  )
}

function EnumOption({ name, stored, set, disabled }: OptionProps) {
  const schema = formSchema(name)
  const values: readonly (string | number)[] =
    schema.kind === "enum" ? schema.values : []
  return (
    <SelectField
      label={optionLabels[name]}
      value={stored === undefined ? "" : String(stored)}
      options={[
        { value: "", label: enumDefaultLabel(name) },
        ...values.map((value) => ({
          value: String(value),
          label: valueLabel(value),
        })),
      ]}
      disabled={disabled}
      onChange={(next) => set(values.find((value) => String(value) === next))}
    />
  )
}

function EntryOption({ name, stored, set, disabled }: OptionProps) {
  const current =
    typeof stored === "number" || typeof stored === "string"
      ? String(stored)
      : ""
  const { entry, edit } = useEntry(current)
  const invalid = !parseOptionEntry(name, entry).ok
  const numeric = formSchema(name).kind === "number"
  return (
    <EntryField
      label={optionLabels[name]}
      hint={invalid ? entryHint(name) : undefined}
      invalid={invalid}
    >
      {(props) => (
        <Input
          {...props}
          inputMode={numeric ? "decimal" : undefined}
          value={entry}
          placeholder={entryPlaceholder(name)}
          readOnly={disabled}
          onChange={(event) => {
            const next = event.target.value
            const parsed = parseOptionEntry(name, next)
            if (!parsed.ok) {
              edit(next, current)
              return
            }
            edit(next, parsed.value === undefined ? "" : String(parsed.value))
            set(parsed.value)
          }}
        />
      )}
    </EntryField>
  )
}

function OptionControl(props: OptionProps) {
  switch (formSchema(props.name).kind) {
    case "toggle":
      return <ToggleOption {...props} />
    case "enum":
      return <EnumOption {...props} />
    case "number":
    case "text":
    case "color":
      return <EntryOption {...props} />
  }
}

function setCount(group: OptionGroup, fields: Record<string, unknown>) {
  return group.keys.filter((key) => fields[key] !== undefined).length
}

function GroupSection({
  group,
  fields,
  dispatch,
  readOnly,
  open,
}: {
  group: OptionGroup
  fields: Record<string, unknown>
  dispatch: Dispatch<DraftAction>
  readOnly: boolean
  open: boolean
}) {
  const count = setCount(group, fields)
  return (
    <details open={open} className="group/options">
      <summary className="flex cursor-pointer list-none items-center gap-1 rounded-control py-1 text-[13px] font-semibold outline-none focus-visible:ring-2 focus-visible:ring-ring [&::-webkit-details-marker]:hidden">
        <ChevronRightIcon
          aria-hidden="true"
          className="size-3.5 shrink-0 text-muted-foreground group-open/options:rotate-90"
        />
        <span>{group.title}</span>
        {count > 0 ? (
          <span className="font-normal text-muted-foreground">{count} set</span>
        ) : null}
      </summary>
      <div className="flex flex-col gap-3.5 pt-2.5">
        {group.keys.map((key) => (
          <OptionControl
            key={key}
            name={key}
            stored={fields[key]}
            disabled={readOnly}
            set={(value) => dispatch({ type: "setOption", key, value })}
          />
        ))}
      </div>
    </details>
  )
}

const jsonOnlyText =
  "Value mappings, links, actions, cell types, dash patterns, gap and null handling, overrides and transformations are edited in JSON."

export function FieldOptions({
  panel,
  dispatch,
  readOnly,
  onOpenJson,
}: {
  panel: DataPanel
  dispatch: Dispatch<DraftAction>
  readOnly: boolean
  onOpenJson?: (() => void) | undefined
}) {
  const fields: Record<string, unknown> = { ...panel }
  return (
    <div className="flex flex-col gap-2.5">
      {optionGroupsFor(panel.type).map((group, index) => (
        <GroupSection
          key={group.id}
          group={group}
          fields={fields}
          dispatch={dispatch}
          readOnly={readOnly}
          open={index === 0 && group.id !== "panel"}
        />
      ))}
      <p className="text-xs text-muted-foreground">{jsonOnlyText}</p>
      {onOpenJson === undefined ? null : (
        <Button
          variant="outline"
          size="sm"
          className="self-start"
          onClick={onOpenJson}
        >
          Edit in JSON
        </Button>
      )}
    </div>
  )
}
