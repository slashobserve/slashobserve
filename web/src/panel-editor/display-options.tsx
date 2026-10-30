import { useId, useState, type Dispatch, type ReactNode } from "react"

import { Input } from "@/components/ui/input"
import {
  displayWithDefaults,
  isDefaultDisplayValue,
  type DisplayPanelType,
} from "@/spec"

import { displayOptions } from "./display-options/index"
import { parseNames, parseNumber, parseTags } from "./display-options/parse"
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
  numeric,
}: {
  controls: Controls
  label: string
  name: string
  options: readonly Option[]
  absent?: string
  numeric?: boolean
}) {
  const stored = controls.display[name]
  return (
    <SelectField
      label={label}
      value={numeric ? String(stored ?? "") : text(controls.display, name)}
      options={
        absent === undefined
          ? options
          : [{ value: "", label: absent }, ...options]
      }
      disabled={controls.disabled}
      onChange={(value) =>
        controls.set(
          name,
          value === "" ? undefined : numeric ? Number(value) : value
        )
      }
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

const bucketModes: readonly Option[] = [
  { value: "", label: "Automatic" },
  { value: "size", label: "Size" },
  { value: "count", label: "Count" },
]

const bucketValueRule: NumberRule = {
  min: 0,
  max: Infinity,
  minExclusive: true,
}

function BucketsOption({
  controls,
  label,
  name,
  sizeHint,
  countHint,
}: {
  controls: Controls
  label: string
  name: string
  sizeHint: string
  countHint: string
}) {
  const stored = controls.display[name] as
    { mode?: string; value?: number } | undefined
  const mode = stored?.mode ?? ""
  const current = typeof stored?.value === "number" ? String(stored.value) : ""
  const { entry, edit } = useEntry(current)
  const trimmed = entry.trim()
  const invalid =
    trimmed !== "" && parseNumber(trimmed, bucketValueRule) === null
  return (
    <>
      <SelectField
        label={`${label} mode`}
        value={mode}
        options={bucketModes}
        disabled={controls.disabled}
        onChange={(next) => {
          if (next === "") {
            controls.set(name, undefined)
            return
          }
          controls.set(name, {
            mode: next,
            ...(typeof stored?.value === "number"
              ? { value: stored.value }
              : {}),
          })
        }}
      />
      {mode === "" ? null : (
        <EntryField
          label={`${label} value`}
          hint={mode === "size" ? sizeHint : countHint}
          invalid={invalid}
        >
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
                const number =
                  nextTrimmed === ""
                    ? undefined
                    : parseNumber(nextTrimmed, bucketValueRule)
                if (number === null) {
                  edit(next, current)
                  return
                }
                edit(next, number === undefined ? "" : String(number))
                controls.set(name, {
                  mode,
                  ...(number === undefined ? {} : { value: number }),
                })
              }}
            />
          )}
        </EntryField>
      )}
    </>
  )
}

const listKinds = {
  tags: {
    parse: parseTags,
    placeholder: "All tags",
    allowed: "Lowercase letters, digits, dots, dashes and underscores.",
  },
  names: {
    parse: parseNames,
    placeholder: "All columns",
    allowed: "Names of up to 200 characters, in any case.",
  },
}

function ListOption({
  controls,
  kind,
  label,
  name,
  max,
  placeholder = listKinds[kind].placeholder,
}: {
  controls: Controls
  kind: keyof typeof listKinds
  label: string
  name: string
  max: number
  placeholder?: string
}) {
  const { parse, allowed } = listKinds[kind]
  const stored = list(controls.display, name).join(", ")
  const { entry, edit } = useEntry(stored)
  const itemsOf = (value: string) => {
    const items = parse(value)
    return items !== null && items.length <= max ? items : null
  }
  const invalid = itemsOf(entry) === null
  const hint = `Separated by commas. ${allowed} At most ${max}.`
  return (
    <EntryField label={label} hint={hint} invalid={invalid}>
      {(props) => (
        <Input
          {...props}
          value={entry}
          placeholder={placeholder}
          readOnly={controls.disabled}
          onChange={(event) => {
            const next = event.target.value
            const items = itemsOf(next)
            if (items === null) {
              edit(next, stored)
              return
            }
            edit(next, items.join(", "))
            controls.set(name, items)
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
          numeric={option.numeric}
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
    case "buckets":
      return (
        <BucketsOption
          controls={controls}
          label={option.label}
          name={option.key}
          sizeHint={option.sizeHint}
          countHint={option.countHint}
        />
      )
    case "tags":
      return (
        <ListOption
          controls={controls}
          kind="tags"
          label={option.label}
          name={option.key}
          max={option.max}
        />
      )
    case "names":
      return (
        <ListOption
          controls={controls}
          kind="names"
          label={option.label}
          name={option.key}
          max={option.max}
          placeholder={option.placeholder}
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
