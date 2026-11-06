import { useState, type FormEvent } from "react"
import { useQuery } from "@tanstack/react-query"
import { XIcon } from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { labelValuesQuery } from "@/panel-editor/queries"

import { pillOptions, type NewPill, type Pill } from "./pills"
import { useLogsWorkspace } from "./workspace"

type FormOperator = "is" | "is not" | "is one of" | "exists" | "between"

const operatorLabels: { value: FormOperator; label: string }[] = [
  { value: "is", label: "is" },
  { value: "is not", label: "is not" },
  { value: "is one of", label: "is one of" },
  { value: "exists", label: "exists" },
  { value: "between", label: "between" },
]
const typedReasons = new Set<string>(["input-change", "input-paste"])

function offeredOperators(type: string) {
  const { kinds } = pillOptions(type)
  return operatorLabels.filter(({ value }) => {
    switch (value) {
      case "is":
      case "is not":
        return kinds.includes("is")
      case "is one of":
        return kinds.includes("oneOf")
      case "exists":
        return kinds.includes("exists")
      case "between":
        return kinds.includes("range")
    }
  })
}

function operatorOf(initial: Pill | undefined): FormOperator {
  if (initial === undefined) return "is"
  switch (initial.kind) {
    case "is":
      return initial.negate ? "is not" : "is"
    case "oneOf":
      return "is one of"
    case "exists":
      return "exists"
    case "range":
      return "between"
  }
}

function boundOf(initial: Pill | undefined, lower: boolean): string {
  if (initial?.kind !== "range") return ""
  const isLower = initial.op === ">" || initial.op === ">="
  return isLower === lower ? (initial.value ?? "") : ""
}

function countText(count: number, truncated: boolean): string {
  const noun = count === 1 ? "value" : "values"
  return `${count.toLocaleString("en")}${truncated ? "+" : ""} ${noun} in the range`
}

function SuggestBox({
  label,
  value,
  onChange,
  onEnter,
  items,
  placeholder,
  autoFocus,
}: {
  label: string
  value: string
  onChange: (next: string) => void
  onEnter?: (event: FormEvent) => void
  items: readonly string[]
  placeholder?: string
  autoFocus?: boolean
}) {
  return (
    <Combobox<string>
      items={items}
      value={null}
      inputValue={value}
      onInputValueChange={(next, details) => {
        if (typedReasons.has(details.reason)) onChange(next)
      }}
      onValueChange={(item) => {
        if (item !== null) onChange(item)
      }}
    >
      <ComboboxInput
        aria-label={label}
        placeholder={placeholder}
        showTrigger={false}
        autoFocus={autoFocus}
        className="w-full"
        onKeyDown={(event) => {
          if (event.key === "Enter" && onEnter !== undefined) onEnter(event)
        }}
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
  )
}

export function FilterForm({
  initial,
  fields,
  onSubmit,
  onCancel,
}: {
  initial?: Pill
  fields: readonly string[]
  onSubmit: (pills: NewPill[]) => void
  onCancel: () => void
}) {
  const { datasource, range } = useLogsWorkspace()
  const operators = offeredOperators(datasource.type)
  const [field, setField] = useState(initial?.field ?? "")
  const [operator, setOperator] = useState<FormOperator>(operatorOf(initial))
  const [value, setValue] = useState(
    initial?.kind === "is" ? (initial.value ?? "") : ""
  )
  const [values, setValues] = useState(
    initial?.kind === "oneOf" ? (initial.values ?? []) : []
  )
  const [lower, setLower] = useState(boundOf(initial, true))
  const [upper, setUpper] = useState(boundOf(initial, false))
  const name = field.trim()
  const oneOf = operator === "is one of"
  const suggestsValues =
    operator === "is" || operator === "is not" || operator === "is one of"
  const known = fields.includes(name)
  const suggestions = useQuery({
    ...labelValuesQuery(datasource.uid, name, {
      from: range.from,
      to: range.to,
    }),
    enabled: suggestsValues && known,
  })
  const pending = value.trim()
  const complete =
    name !== "" &&
    (operator === "exists" ||
      (operator === "between"
        ? lower.trim() !== "" || upper.trim() !== ""
        : oneOf
          ? values.length > 0 || pending !== ""
          : pending !== ""))

  function addPendingValue() {
    if (pending !== "" && !values.includes(pending)) {
      setValues([...values, pending])
    }
    setValue("")
  }

  function submit(event: FormEvent) {
    event.preventDefault()
    if (oneOf && pending !== "") {
      addPendingValue()
      return
    }
    if (!complete) return
    const kept = {
      field: name,
      pinned: initial?.pinned ?? false,
      paused: initial?.paused ?? false,
    }
    switch (operator) {
      case "is":
      case "is not":
        onSubmit([
          {
            ...kept,
            kind: "is",
            value: pending,
            negate: operator === "is not",
          },
        ])
        return
      case "is one of":
        onSubmit([{ ...kept, kind: "oneOf", values, negate: false }])
        return
      case "exists":
        onSubmit([
          {
            ...kept,
            kind: "exists",
            negate: initial?.kind === "exists" && initial.negate,
          },
        ])
        return
      case "between": {
        const bounds: NewPill[] = []
        if (lower.trim() !== "") {
          bounds.push({
            ...kept,
            kind: "range",
            op: ">=",
            value: lower.trim(),
            negate: false,
          })
        }
        if (upper.trim() !== "") {
          bounds.push({
            ...kept,
            kind: "range",
            op: "<=",
            value: upper.trim(),
            negate: false,
          })
        }
        onSubmit(bounds)
      }
    }
  }

  return (
    <form onSubmit={submit} className="flex flex-col gap-2.5">
      <div className="flex flex-col gap-1.5">
        <Label>Field</Label>
        <SuggestBox
          label="Field"
          value={field}
          onChange={setField}
          items={fields}
          placeholder="service.name"
          autoFocus
        />
      </div>
      <div className="flex flex-col gap-1.5">
        <Label>Operator</Label>
        <Select
          items={operators}
          value={operator}
          onValueChange={(next) => {
            if (next !== null) setOperator(next)
          }}
        >
          <SelectTrigger aria-label="Operator" className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent alignItemWithTrigger={false}>
            {operators.map((item) => (
              <SelectItem key={item.value} value={item.value}>
                {item.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      {operator === "exists" ? null : operator === "between" ? (
        <div className="grid grid-cols-2 gap-2">
          <div className="flex flex-col gap-1.5">
            <Label>At least</Label>
            <Input
              aria-label="At least"
              value={lower}
              onChange={(event) => setLower(event.target.value)}
              placeholder="400"
            />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label>At most</Label>
            <Input
              aria-label="At most"
              value={upper}
              onChange={(event) => setUpper(event.target.value)}
              placeholder="599"
            />
          </div>
        </div>
      ) : (
        <div className="flex flex-col gap-1.5">
          <Label>{oneOf ? "Values" : "Value"}</Label>
          {oneOf && values.length > 0 ? (
            <ul className="m-0 flex list-none flex-wrap gap-1 p-0">
              {values.map((item) => (
                <li key={item}>
                  <Badge variant="outline" className="gap-0.5 pr-0.5">
                    {item}
                    <button
                      type="button"
                      aria-label={`Remove value ${item}`}
                      className="inline-flex size-4 items-center justify-center rounded-control hover:bg-muted"
                      onClick={() =>
                        setValues(values.filter((other) => other !== item))
                      }
                    >
                      <XIcon aria-hidden="true" />
                    </button>
                  </Badge>
                </li>
              ))}
            </ul>
          ) : null}
          <SuggestBox
            label={oneOf ? "Add a value" : "Value"}
            value={value}
            onChange={setValue}
            onEnter={
              oneOf && pending !== ""
                ? (event) => {
                    event.preventDefault()
                    addPendingValue()
                  }
                : undefined
            }
            items={suggestions.data?.values ?? []}
            placeholder={oneOf ? "Type a value, press Enter" : "checkout"}
          />
          {suggestions.data !== undefined ? (
            <p className="text-xs text-muted-foreground">
              {countText(
                suggestions.data.values.length,
                suggestions.data.truncated
              )}
            </p>
          ) : null}
        </div>
      )}
      <div className="flex justify-end gap-2">
        <Button type="button" variant="ghost" size="sm" onClick={onCancel}>
          Cancel
        </Button>
        <Button type="submit" size="sm" disabled={!complete}>
          {initial === undefined ? "Add filter" : "Save"}
        </Button>
      </div>
    </form>
  )
}
