import { useId, useState, type Dispatch, type ReactNode } from "react"

import { Input } from "@/components/ui/input"
import {
  displayWithDefaults,
  isDefaultDisplayValue,
  type DisplayPanelType,
} from "@/spec"

import { displayOptions } from "./display-options/index"
import { parseNumber, parseTags } from "./display-options/parse"
import { reducerOptions } from "./display-options/shared"
import type { DisplayOption, NumberRule } from "./display-options/types"
import type { DraftAction } from "./draft"
import {
  CheckboxField,
  CheckboxGroup,
  Field,
  SelectField,
  type Option,
} from "./field-controls"
import { toggleOrdered } from "./ordered-set"

type Controls = {
  display: Record<string, unknown>
  set: (key: string, value: unknown) => void
  disabled: boolean
}

function text(display: Record<string, unknown>, key: string): string {
  const value = display[key]
  return typeof value === "string" ? value : ""
}

function flag(display: Record<string, unknown>, key: string): boolean {
  return display[key] === true
}

function list(display: Record<string, unknown>, key: string): string[] {
  const value = display[key]
  return Array.isArray(value)
    ? value.filter((item): item is string => typeof item === "string")
    : []
}

function Choice({
  controls,
  label,
  name,
  options,
  absent,
}: {
  controls: Controls
  label: string
  name: string
  options: readonly Option[]
  absent?: string
}) {
  return (
    <SelectField
      label={label}
      value={text(controls.display, name)}
      options={
        absent === undefined
          ? options
          : [{ value: "", label: absent }, ...options]
      }
      disabled={controls.disabled}
      onChange={(value) => controls.set(name, value === "" ? undefined : value)}
    />
  )
}

function Toggle({
  controls,
  label,
  name,
}: {
  controls: Controls
  label: string
  name: string
}) {
  return (
    <CheckboxField
      label={label}
      checked={flag(controls.display, name)}
      disabled={controls.disabled}
      onChange={(checked) => controls.set(name, checked)}
    />
  )
}

function Reducers({
  controls,
  label,
  name,
  options,
  max,
}: {
  controls: Controls
  label: string
  name: string
  options: readonly Option[]
  max?: number
}) {
  const selected = list(controls.display, name)
  return (
    <CheckboxGroup
      label={label}
      options={options}
      selected={selected}
      max={max}
      disabled={controls.disabled}
      onToggle={(value, on) =>
        controls.set(
          name,
          toggleOrdered(
            options.map((option) => option.value),
            selected,
            value,
            on
          )
        )
      }
    />
  )
}

function useEntry(stored: string) {
  const [entry, setEntry] = useState(stored)
  const [synced, setSynced] = useState(stored)
  if (stored !== synced) {
    setSynced(stored)
    setEntry(stored)
  }
  return {
    entry,
    edit: (next: string, committed: string) => {
      setEntry(next)
      setSynced(committed)
    },
  }
}

function EntryField({
  label,
  hint,
  invalid,
  children,
}: {
  label: string
  hint?: string
  invalid: boolean
  children: (props: {
    id: string
    "aria-invalid": boolean
    "aria-describedby"?: string
  }) => ReactNode
}) {
  const id = useId()
  return (
    <Field label={label} htmlFor={id}>
      {children({
        id,
        "aria-invalid": invalid,
        "aria-describedby": hint === undefined ? undefined : `${id}-hint`,
      })}
      {hint === undefined ? null : (
        <span
          id={`${id}-hint`}
          className={
            invalid
              ? "text-xs text-status-critical"
              : "text-xs text-muted-foreground"
          }
        >
          {hint}
        </span>
      )}
    </Field>
  )
}

function NumberOption({
  controls,
  label,
  name,
  hint,
  rule,
}: {
  controls: Controls
  label: string
  name: string
  hint: string
  rule: NumberRule
}) {
  const stored = controls.display[name]
  const current = typeof stored === "number" ? String(stored) : ""
  const { entry, edit } = useEntry(current)
  const parse = (value: string) => parseNumber(value, rule)
  const trimmed = entry.trim()
  const invalid = trimmed !== "" && parse(trimmed) === null
  return (
    <EntryField label={label} hint={hint} invalid={invalid}>
      {(props) => (
        <Input
          {...props}
          inputMode="decimal"
          value={entry}
          placeholder="Auto"
          readOnly={controls.disabled}
          onChange={(event) => {
            const next = event.target.value
            const nextTrimmed = next.trim()
            const number = nextTrimmed === "" ? undefined : parse(nextTrimmed)
            if (number === null) {
              edit(next, current)
              return
            }
            edit(next, number === undefined ? "" : String(number))
            controls.set(name, number)
          }}
        />
      )}
    </EntryField>
  )
}

function TextOption({
  controls,
  label,
  name,
  placeholder,
  maxLength,
}: {
  controls: Controls
  label: string
  name: string
  placeholder: string
  maxLength: number
}) {
  const { entry, edit } = useEntry(text(controls.display, name))
  return (
    <EntryField label={label} invalid={false}>
      {(props) => (
        <Input
          {...props}
          value={entry}
          maxLength={maxLength}
          placeholder={placeholder}
          readOnly={controls.disabled}
          onChange={(event) => {
            const next = event.target.value
            const value = next.trim()
            edit(next, value)
            controls.set(name, value === "" ? undefined : value)
          }}
        />
      )}
    </EntryField>
  )
}

function TagsOption({
  controls,
  label,
  name,
  max,
}: {
  controls: Controls
  label: string
  name: string
  max: number
}) {
  const stored = list(controls.display, name).join(", ")
  const { entry, edit } = useEntry(stored)
  const tagsOf = (value: string) => {
    const tags = parseTags(value)
    return tags !== null && tags.length <= max ? tags : null
  }
  const invalid = tagsOf(entry) === null
  const hint = `Separated by commas. Lowercase letters, digits, dots, dashes and underscores. At most ${max}.`
  return (
    <EntryField label={label} hint={hint} invalid={invalid}>
      {(props) => (
        <Input
          {...props}
          value={entry}
          placeholder="All tags"
          readOnly={controls.disabled}
          onChange={(event) => {
            const next = event.target.value
            const tags = tagsOf(next)
            if (tags === null) {
              edit(next, stored)
              return
            }
            edit(next, tags.join(", "))
            controls.set(name, tags)
          }}
        />
      )}
    </EntryField>
  )
}

function OptionControl({
  option,
  controls,
}: {
  option: DisplayOption
  controls: Controls
}) {
  switch (option.kind) {
    case "choice":
      return (
        <Choice
          controls={controls}
          label={option.label}
          name={option.key}
          options={option.options}
          absent={option.absent}
        />
      )
    case "toggle":
      return (
        <Toggle controls={controls} label={option.label} name={option.key} />
      )
    case "reducer":
      return (
        <Choice
          controls={controls}
          label={option.label}
          name={option.key}
          options={reducerOptions}
        />
      )
    case "reducers":
      return (
        <Reducers
          controls={controls}
          label={option.label}
          name={option.key}
          options={option.options}
          max={option.max}
        />
      )
    case "number":
      return (
        <NumberOption
          controls={controls}
          label={option.label}
          name={option.key}
          hint={option.hint}
          rule={option.rule}
        />
      )
    case "text":
      return (
        <TextOption
          controls={controls}
          label={option.label}
          name={option.key}
          placeholder={option.placeholder}
          maxLength={option.maxLength}
        />
      )
    case "tags":
      return (
        <TagsOption
          controls={controls}
          label={option.label}
          name={option.key}
          max={option.max}
        />
      )
    case "note":
      return <p className="text-xs text-muted-foreground">{option.text}</p>
  }
}

export function DisplayOptions({
  type,
  display,
  dispatch,
  readOnly,
}: {
  type: DisplayPanelType
  display: unknown
  dispatch: Dispatch<DraftAction>
  readOnly: boolean
}) {
  const options = displayOptions[type]
  if (options.length === 0) return null
  const controls: Controls = {
    display: displayWithDefaults(type, display),
    disabled: readOnly,
    set: (key, value) =>
      dispatch({
        type: "setDisplay",
        key,
        value:
          value === undefined || isDefaultDisplayValue(type, key, value)
            ? undefined
            : value,
      }),
  }
  return (
    <div className="flex flex-col gap-3.5">
      <h3 className="pt-1 text-[13px] font-semibold">Display</h3>
      {options.map((option, index) => (
        <OptionControl
          key={"key" in option ? option.key : index}
          option={option}
          controls={controls}
        />
      ))}
    </div>
  )
}
