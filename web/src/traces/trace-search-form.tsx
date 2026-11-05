import { useId, useState, type FormEvent } from "react"
import { useQuery } from "@tanstack/react-query"

import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { labelValuesQuery } from "@/panel-editor/queries"

import type { TraceSearch } from "./search"
import { parseDuration } from "./search-query"

type FormSearch = Pick<
  TraceSearch,
  "service" | "span" | "min" | "max" | "errors" | "q"
>

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
  id,
  label,
  value,
  onChange,
  placeholder,
  problem,
  list,
}: {
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
  type,
  uid,
  value,
  onSubmit,
}: {
  type: string
  uid: string
  value: TraceSearch
  onSubmit: (next: FormSearch) => void
}) {
  const ids = useId()
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
      aria-label="Search traces"
      noValidate
      onSubmit={submit}
      className="flex flex-col gap-3"
    >
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <TextField
          id={`${ids}-service`}
          label="Service"
          value={service}
          onChange={setService}
          list={clickhouse ? `${ids}-services` : undefined}
        />
        <TextField
          id={`${ids}-span`}
          label="Span name"
          value={span}
          onChange={setSpan}
        />
        <TextField
          id={`${ids}-min`}
          label="Min duration"
          value={min}
          onChange={setMin}
          placeholder="250ms"
          problem={minProblem}
        />
        <TextField
          id={`${ids}-max`}
          label="Max duration"
          value={max}
          onChange={setMax}
          placeholder="250ms"
          problem={maxProblem}
        />
      </div>
      {clickhouse ? <ServiceOptions id={`${ids}-services`} uid={uid} /> : null}
      <Label className="gap-2 text-[13px] font-normal">
        <Checkbox
          checked={errors}
          onCheckedChange={(next) => setErrors(next)}
        />
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
            onChange={(event) => setQ(event.target.value)}
          />
          <p id={`${ids}-q-help`} className="text-xs text-muted-foreground">
            Leave empty to search with the fields above.
          </p>
        </div>
      ) : null}
      <div>
        <Button type="submit">Search traces</Button>
      </div>
    </form>
  )
}
