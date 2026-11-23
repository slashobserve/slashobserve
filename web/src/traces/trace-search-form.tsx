import { useId, useRef, useState, type FormEvent, type Ref } from "react"
import { useQuery } from "@tanstack/react-query"

import { Checkbox } from "@/components/ui/checkbox"
import {
  Combobox,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxInput,
  ComboboxItem,
  ComboboxList,
} from "@/components/ui/combobox"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { languageReference } from "@/datasources/sources"
import { labelValuesQuery } from "@/panel-editor/queries"

import { hasQueryText, type TraceFormSearch, type TraceSearch } from "./search"
import { parseDuration } from "./search-query"
import { queryLabel, traceFilterLabels } from "./trace-filters"

const durationProblem = "Use a number and a unit, like 250ms or 1.5s."
const maxNameLength = 200
const maxQueryLength = 8000

function optional(text: string): string | undefined {
  const trimmed = text.trim()
  return trimmed === "" ? undefined : trimmed
}

function durationInvalid(text: string): boolean {
  const trimmed = text.trim()
  return trimmed !== "" && parseDuration(trimmed) === null
}

function TextField({
  ref,
  id,
  label,
  value,
  onChange,
  placeholder,
  problem,
  disabled,
}: {
  ref?: Ref<HTMLInputElement>
  id: string
  label: string
  value: string
  onChange: (value: string) => void
  placeholder?: string
  problem?: string | null
  disabled?: boolean
}) {
  return (
    <div className="flex flex-col gap-1.5">
      <Label htmlFor={id} className="text-[13px]">
        {label}
      </Label>
      <Input
        ref={ref}
        id={id}
        value={value}
        placeholder={placeholder}
        maxLength={maxNameLength}
        disabled={disabled}
        aria-invalid={problem ? true : undefined}
        aria-describedby={problem ? `${id}-problem` : undefined}
        onChange={(event) => onChange(event.target.value)}
      />
      {problem ? (
        <p id={`${id}-problem`} className="text-xs text-destructive">
          {problem}
        </p>
      ) : null}
    </div>
  )
}

const typedReasons = new Set<string>(["input-change", "input-paste", "clear"])

function ServiceCombobox({
  ref,
  id,
  value,
  onChange,
  uid,
  type,
  disabled,
}: {
  ref?: Ref<HTMLInputElement>
  id: string
  value: string
  onChange: (value: string) => void
  uid: string
  type: string
  disabled: boolean
}) {
  const services = useQuery(
    type === "jaeger"
      ? labelValuesQuery(uid, "service", {})
      : labelValuesQuery(uid, "ServiceName", { table: "otel_traces" })
  )
  return (
    <div className="flex flex-col gap-1.5">
      <Label htmlFor={id} className="text-[13px]">
        {traceFilterLabels.service}
      </Label>
      <Combobox<string>
        items={services.data?.values ?? []}
        value={null}
        inputValue={value}
        disabled={disabled}
        onInputValueChange={(next, details) => {
          if (typedReasons.has(details.reason)) onChange(next)
        }}
        onValueChange={(item) => {
          if (item !== null) onChange(item)
        }}
      >
        <ComboboxInput
          ref={ref}
          id={id}
          maxLength={maxNameLength}
          disabled={disabled}
          showClear
          className="w-full"
        />
        <ComboboxContent>
          <ComboboxEmpty>No suggestions</ComboboxEmpty>
          <ComboboxList>
            {(item: string) => (
              <ComboboxItem key={item} value={item}>
                {item}
              </ComboboxItem>
            )}
          </ComboboxList>
        </ComboboxContent>
      </Combobox>
    </div>
  )
}

export function TraceSearchForm({
  id,
  type,
  uid,
  value,
  onSubmit,
  answered = false,
  onEdited,
  serviceRef,
}: {
  id: string
  type: string
  uid: string
  value: TraceSearch
  onSubmit: (next: TraceFormSearch) => void
  answered?: boolean
  onEdited?: () => void
  serviceRef?: Ref<HTMLInputElement>
}) {
  const ids = useId()
  const editReported = useRef(false)
  const [service, setService] = useState(value.service ?? "")
  const [span, setSpan] = useState(value.span ?? "")
  const [min, setMin] = useState(value.min ?? "")
  const [max, setMax] = useState(value.max ?? "")
  const [errors, setErrors] = useState(value.errors === "1")
  const [q, setQ] = useState(value.q ?? "")
  const [checked, setChecked] = useState(false)
  const queryText = hasQueryText(type)
  const reference = languageReference("traces", type)
  const services = type !== "tempo"
  const queryReplacesFields = queryText && q.trim() !== ""
  const minProblem =
    checked && !queryReplacesFields && durationInvalid(min)
      ? durationProblem
      : null
  const maxProblem =
    checked && !queryReplacesFields && durationInvalid(max)
      ? durationProblem
      : null

  function edited<Value>(set: (next: Value) => void) {
    return (next: Value) => {
      set(next)
      if (!answered || editReported.current) return
      editReported.current = true
      onEdited?.()
    }
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setChecked(true)
    if (queryReplacesFields) {
      onSubmit({ q: optional(q) })
      return
    }
    if (durationInvalid(min) || durationInvalid(max)) return
    onSubmit({
      service: optional(service),
      span: optional(span),
      min: optional(min),
      max: optional(max),
      errors: errors ? "1" : undefined,
      q: queryText ? optional(q) : undefined,
    })
  }

  return (
    <form
      id={id}
      aria-label="Search traces"
      noValidate
      onSubmit={submit}
      className="flex flex-col gap-3"
    >
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
        {services ? (
          <ServiceCombobox
            ref={serviceRef}
            id={`${ids}-service`}
            value={service}
            onChange={edited(setService)}
            uid={uid}
            type={type}
            disabled={queryReplacesFields}
          />
        ) : (
          <TextField
            ref={serviceRef}
            id={`${ids}-service`}
            label={traceFilterLabels.service}
            value={service}
            onChange={edited(setService)}
            disabled={queryReplacesFields}
          />
        )}
        <TextField
          id={`${ids}-span`}
          label={traceFilterLabels.span}
          value={span}
          onChange={edited(setSpan)}
          disabled={queryReplacesFields}
        />
        <TextField
          id={`${ids}-min`}
          label={traceFilterLabels.min}
          value={min}
          onChange={edited(setMin)}
          placeholder="250ms"
          problem={minProblem}
          disabled={queryReplacesFields}
        />
        <TextField
          id={`${ids}-max`}
          label={traceFilterLabels.max}
          value={max}
          onChange={edited(setMax)}
          placeholder="2s"
          problem={maxProblem}
          disabled={queryReplacesFields}
        />
      </div>
      <Label className="gap-2 text-[13px] font-normal">
        <Checkbox
          checked={errors}
          disabled={queryReplacesFields}
          onCheckedChange={edited(setErrors)}
        />
        {traceFilterLabels.errors}
      </Label>
      {queryText ? (
        <div className="flex flex-col gap-1.5">
          <Label htmlFor={`${ids}-q`} className="text-[13px]">
            {queryLabel(type)}
          </Label>
          <Textarea
            id={`${ids}-q`}
            value={q}
            rows={2}
            maxLength={maxQueryLength}
            aria-describedby={`${ids}-q-help`}
            className="font-mono"
            onChange={(event) => edited(setQ)(event.target.value)}
          />
          <p id={`${ids}-q-help`} className="text-xs text-muted-foreground">
            {queryReplacesFields
              ? "This query replaces the fields above; clear it to use them again."
              : "Leave empty to search with the fields above."}{" "}
            {reference === null ? null : (
              <a
                href={reference.url}
                target="_blank"
                rel="noreferrer"
                className="underline underline-offset-3"
              >
                {reference.label}
              </a>
            )}
          </p>
        </div>
      ) : null}
    </form>
  )
}
