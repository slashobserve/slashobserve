import { useId, useState, type Dispatch } from "react"

import { Input } from "@/components/ui/input"
import {
  displayWithDefaults,
  isDefaultDisplayValue,
  type DataPanelType,
} from "@/spec"

import type { DraftAction } from "./draft"
import {
  CheckboxField,
  CheckboxGroup,
  Field,
  SelectField,
  type Option,
} from "./field-controls"
import { toggleOrdered } from "./ordered-set"

const reducerOptions: Option[] = [
  { value: "lastNotNull", label: "Last" },
  { value: "last", label: "Last, including empty" },
  { value: "first", label: "First" },
  { value: "firstNotNull", label: "First, not empty" },
  { value: "min", label: "Min" },
  { value: "max", label: "Max" },
  { value: "mean", label: "Mean" },
  { value: "median", label: "Median" },
  { value: "sum", label: "Total" },
  { value: "count", label: "Count" },
  { value: "range", label: "Range" },
  { value: "delta", label: "Delta" },
  { value: "diff", label: "Difference" },
  { value: "stdDev", label: "Standard deviation" },
  { value: "distinctCount", label: "Distinct values" },
  { value: "changeCount", label: "Changes" },
]

const legendOptions: Option[] = [
  { value: "bottom", label: "Bottom" },
  { value: "right", label: "Right" },
  { value: "hidden", label: "Hidden" },
]

const orientationOptions: Option[] = [
  { value: "auto", label: "Auto" },
  { value: "horizontal", label: "Horizontal" },
  { value: "vertical", label: "Vertical" },
]

const legendValueOptions: Option[] = [
  { value: "lastNotNull", label: "Last" },
  { value: "mean", label: "Mean" },
  { value: "max", label: "Max" },
  { value: "min", label: "Min" },
  { value: "sum", label: "Total" },
]

const footerOptions: Option[] = [
  { value: "sum", label: "Total" },
  { value: "mean", label: "Mean" },
  { value: "min", label: "Min" },
  { value: "max", label: "Max" },
  { value: "lastNotNull", label: "Last" },
  { value: "count", label: "Count" },
]

const maxFooterReducers = 5
const maxBucketCount = 1000

type Controls = {
  type: DataPanelType
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

function Calculation({ controls }: { controls: Controls }) {
  return (
    <Choice
      controls={controls}
      label="Calculation"
      name="reduce"
      options={reducerOptions}
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

function NumberOption({
  controls,
  label,
  name,
  hint,
  parse,
}: {
  controls: Controls
  label: string
  name: string
  hint: string
  parse: (text: string) => number | null
}) {
  const id = useId()
  const stored = controls.display[name]
  const value = typeof stored === "number" ? stored : undefined
  const [entry, setEntry] = useState(value === undefined ? "" : String(value))
  const [synced, setSynced] = useState(value)
  if (value !== synced) {
    setSynced(value)
    setEntry(value === undefined ? "" : String(value))
  }
  const parsed = entry.trim() === "" ? undefined : parse(entry.trim())
  const invalid = parsed === null
  return (
    <Field label={label} htmlFor={id}>
      <Input
        id={id}
        inputMode="decimal"
        value={entry}
        placeholder="Auto"
        aria-invalid={invalid}
        aria-describedby={`${id}-hint`}
        readOnly={controls.disabled}
        onChange={(event) => {
          const next = event.target.value
          setEntry(next)
          const number = next.trim() === "" ? undefined : parse(next.trim())
          if (number === null) return
          setSynced(number)
          controls.set(name, number)
        }}
      />
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
    </Field>
  )
}

function parseBucketSize(entry: string): number | null {
  if (!/^(\d+\.?\d*|\.\d+)(e[+-]?\d+)?$/i.test(entry)) return null
  const value = Number(entry)
  return Number.isFinite(value) && value > 0 ? value : null
}

function parseBucketCount(entry: string): number | null {
  if (!/^\d+$/.test(entry)) return null
  const value = Number(entry)
  return value >= 1 && value <= maxBucketCount ? value : null
}

function TypeOptions({ controls }: { controls: Controls }) {
  switch (controls.type) {
    case "timeseries":
      return (
        <>
          <Choice
            controls={controls}
            label="Legend"
            name="legend"
            options={legendOptions}
          />
          <Reducers
            controls={controls}
            label="Legend values"
            name="legendValues"
            options={legendValueOptions}
          />
          <Choice
            controls={controls}
            label="Tooltip"
            name="tooltip"
            options={[
              { value: "single", label: "Single series" },
              { value: "all", label: "All series" },
              { value: "hidden", label: "Hidden" },
            ]}
          />
        </>
      )
    case "stat":
      return (
        <>
          <Calculation controls={controls} />
          <Choice
            controls={controls}
            label="Color mode"
            name="colorMode"
            options={[
              { value: "value", label: "Value" },
              { value: "background", label: "Background" },
              { value: "none", label: "None" },
            ]}
          />
          <Toggle controls={controls} label="Show graph" name="graph" />
          <Choice
            controls={controls}
            label="Text"
            name="textMode"
            absent="Auto"
            options={[
              { value: "value", label: "Value" },
              { value: "name", label: "Name" },
              { value: "valueAndName", label: "Value and name" },
              { value: "none", label: "None" },
            ]}
          />
          <Choice
            controls={controls}
            label="Orientation"
            name="orientation"
            options={orientationOptions}
          />
        </>
      )
    case "gauge":
      return (
        <>
          <Calculation controls={controls} />
          <Toggle
            controls={controls}
            label="Show threshold markers"
            name="thresholdMarkers"
          />
        </>
      )
    case "bargauge":
      return (
        <>
          <Calculation controls={controls} />
          <Choice
            controls={controls}
            label="Orientation"
            name="orientation"
            options={orientationOptions}
          />
        </>
      )
    case "table":
      return (
        <>
          <Toggle controls={controls} label="Show header" name="header" />
          <Reducers
            controls={controls}
            label="Footer"
            name="footer"
            options={footerOptions}
            max={maxFooterReducers}
          />
        </>
      )
    case "piechart":
      return (
        <>
          <Calculation controls={controls} />
          <Choice
            controls={controls}
            label="Pie type"
            name="pieType"
            options={[
              { value: "pie", label: "Pie" },
              { value: "donut", label: "Donut" },
            ]}
          />
          <Choice
            controls={controls}
            label="Legend"
            name="legend"
            options={legendOptions}
          />
        </>
      )
    case "heatmap":
      return (
        <>
          <Toggle
            controls={controls}
            label="Calculate buckets"
            name="calculate"
          />
          <Choice
            controls={controls}
            label="Y scale"
            name="yScale"
            options={[
              { value: "linear", label: "Linear" },
              { value: "log", label: "Log" },
            ]}
          />
        </>
      )
    case "histogram":
      return (
        <>
          <NumberOption
            controls={controls}
            label="Bucket size"
            name="bucketSize"
            hint="A number above 0. Empty is automatic."
            parse={parseBucketSize}
          />
          <NumberOption
            controls={controls}
            label="Bucket count"
            name="bucketCount"
            hint="A whole number from 1 to 1000. Empty is automatic."
            parse={parseBucketCount}
          />
        </>
      )
    case "state-timeline":
      return (
        <>
          <Toggle
            controls={controls}
            label="Merge equal values"
            name="mergeValues"
          />
          <Choice
            controls={controls}
            label="Show values"
            name="showValues"
            options={[
              { value: "auto", label: "Auto" },
              { value: "always", label: "Always" },
              { value: "never", label: "Never" },
            ]}
          />
        </>
      )
    case "logs":
      return (
        <>
          <Toggle controls={controls} label="Wrap lines" name="wrap" />
          <Toggle controls={controls} label="Show time" name="showTime" />
          <Choice
            controls={controls}
            label="Order"
            name="order"
            options={[
              { value: "newest", label: "Newest first" },
              { value: "oldest", label: "Oldest first" },
            ]}
          />
        </>
      )
  }
}

export function DisplayOptions({
  type,
  display,
  dispatch,
  readOnly,
}: {
  type: DataPanelType
  display: unknown
  dispatch: Dispatch<DraftAction>
  readOnly: boolean
}) {
  const controls: Controls = {
    type,
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
      <TypeOptions controls={controls} />
    </div>
  )
}
