import { useId, useRef, useState, type FormEvent, type Ref } from "react"
import { useQuery } from "@tanstack/react-query"

import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { labelValuesQuery } from "@/panel-editor/queries"

import type { TraceFormSearch, TraceSearch } from "./search"
import { parseDuration } from "./search-query"

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
  list,
}: {
  ref?: Ref<HTMLInputElement>
  id: string
  label: string
  value: string
  onChange: (value: string) => void
  placeholder?: string
  problem?: string | null
  list?: string
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
        list={list}
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

function ServiceOptions({ id, uid }: { id: string; uid: string }) {
  const services = useQuery(
    labelValuesQuery(uid, "ServiceName", { table: "otel_traces" })
  )
  return (
    <datalist id={id}>
      {services.data?.values.map((service) => (
        <option key={service} value={service} />
      ))}
    </datalist>
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
  const tempo = type === "tempo"
  const clickhouse = type === "clickhouse"
  const minProblem = checked && durationInvalid(min) ? durationProblem : null
  const maxProblem = checked && durationInvalid(max) ? durationProblem : null

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
    if (durationInvalid(min) || durationInvalid(max)) return
    onSubmit({
      service: optional(service),
      span: optional(span),
      min: optional(min),
      max: optional(max),
      errors: errors ? "1" : undefined,
      q: tempo ? optional(q) : undefined,
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
        <TextField
          ref={serviceRef}
          id={`${ids}-service`}
          label="Service"
          value={service}
          onChange={edited(setService)}
          list={clickhouse ? `${ids}-services` : undefined}
        />
        <TextField
          id={`${ids}-span`}
          label="Span name"
          value={span}
          onChange={edited(setSpan)}
        />
        <TextField
          id={`${ids}-min`}
          label="Min duration"
          value={min}
          onChange={edited(setMin)}
          placeholder="250ms"
          problem={minProblem}
        />
        <TextField
          id={`${ids}-max`}
          label="Max duration"
          value={max}
          onChange={edited(setMax)}
          placeholder="2s"
          problem={maxProblem}
        />
      </div>
      {clickhouse ? <ServiceOptions id={`${ids}-services`} uid={uid} /> : null}
      <Label className="gap-2 text-[13px] font-normal">
        <Checkbox checked={errors} onCheckedChange={edited(setErrors)} />
        Errors only
      </Label>
      {tempo ? (
        <div className="flex flex-col gap-1.5">
          <Label htmlFor={`${ids}-q`} className="text-[13px]">
            TraceQL
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
            Leave empty to search with the fields above.
          </p>
        </div>
      ) : null}
    </form>
  )
}
