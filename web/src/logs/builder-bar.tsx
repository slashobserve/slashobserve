import { useState, type FormEvent } from "react"
import { useQuery } from "@tanstack/react-query"
import { PlusIcon, XIcon } from "lucide-react"

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
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { labelNamesQuery, labelValuesQuery } from "@/panel-editor/queries"

import {
  rowLabel,
  rowsOf,
  textOf,
  type BuilderOp,
  type BuilderRow,
} from "./builder"
import { useLogsWorkspace } from "./workspace"

const operators: { value: BuilderOp; label: string }[] = [
  { value: "is", label: "is" },
  { value: "is not", label: "is not" },
  { value: "is one of", label: "is one of" },
  { value: "exists", label: "exists" },
  { value: "gt", label: "is greater than" },
  { value: "gte", label: "is greater than or equal to" },
  { value: "lt", label: "is less than" },
  { value: "lte", label: "is less than or equal to" },
  { value: "contains", label: "contains" },
]
const typedReasons = new Set<string>(["input-change", "input-paste"])
const suggestsValues = new Set<BuilderOp>(["is", "is not", "is one of"])
const needsNoField = new Set<BuilderOp>(["contains"])
const needsNoValue = new Set<BuilderOp>(["exists", "is one of"])

function SuggestBox({
  label,
  value,
  onChange,
  onEnter,
  items,
  placeholder,
}: {
  label: string
  value: string
  onChange: (next: string) => void
  onEnter?: (event: FormEvent) => void
  items: readonly string[]
  placeholder?: string
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

function RowForm({
  initial,
  submitLabel,
  onSubmit,
}: {
  initial: BuilderRow
  submitLabel: string
  onSubmit: (row: BuilderRow) => void
}) {
  const { datasource } = useLogsWorkspace()
  const uid = datasource.uid
  const [field, setField] = useState(initial.field)
  const [op, setOp] = useState<BuilderOp>(initial.op)
  const [value, setValue] = useState(initial.value)
  const [values, setValues] = useState(initial.values)
  const names = useQuery(labelNamesQuery(uid, {}))
  const wantsValues = suggestsValues.has(op) && field.trim() !== ""
  const knownField = names.data?.labels.includes(field.trim()) === true
  const suggestions = useQuery({
    ...labelValuesQuery(uid, field.trim(), {}),
    enabled: wantsValues && knownField,
  })
  const oneOf = op === "is one of"
  const pending = value.trim()
  const complete =
    (needsNoField.has(op) || field.trim() !== "") &&
    (oneOf
      ? values.length > 0 || pending !== ""
      : needsNoValue.has(op) || pending !== "")

  function addPendingValue() {
    if (pending === "" || values.includes(pending)) {
      setValue("")
      return
    }
    setValues([...values, pending])
    setValue("")
  }

  function submit(event: FormEvent) {
    event.preventDefault()
    if (oneOf && pending !== "") {
      addPendingValue()
      return
    }
    if (!complete) return
    onSubmit({
      ...initial,
      field: needsNoField.has(op) ? "" : field.trim(),
      op,
      value: needsNoValue.has(op) ? "" : pending,
      values: oneOf ? values : [],
    })
  }

  return (
    <form onSubmit={submit} className="flex flex-col gap-2.5">
      {needsNoField.has(op) ? null : (
        <div className="flex flex-col gap-1.5">
          <Label>Field</Label>
          <SuggestBox
            label="Field"
            value={field}
            onChange={setField}
            items={names.data?.labels ?? []}
            placeholder="log.level"
          />
        </div>
      )}
      <div className="flex flex-col gap-1.5">
        <Label>Operator</Label>
        <Select
          items={operators}
          value={op}
          onValueChange={(next) => {
            if (next !== null) setOp(next)
          }}
        >
          <SelectTrigger aria-label="Operator" className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent alignItemWithTrigger={false}>
            {operators.map((operator) => (
              <SelectItem key={operator.value} value={operator.value}>
                {operator.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      {op === "exists" ? null : (
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
          {wantsValues ? (
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
              placeholder={oneOf ? "Type a value, press Enter" : "error"}
            />
          ) : (
            <Input
              aria-label="Value"
              value={value}
              onChange={(event) => setValue(event.target.value)}
              placeholder={op === "contains" ? "timeout" : "250"}
            />
          )}
        </div>
      )}
      <div className="flex justify-end">
        <Button type="submit" size="sm" disabled={!complete}>
          {submitLabel}
        </Button>
      </div>
    </form>
  )
}

function FilterChip({
  row,
  onSave,
  onRemove,
}: {
  row: BuilderRow
  onSave: (row: BuilderRow) => void
  onRemove: () => void
}) {
  const [open, setOpen] = useState(false)
  const label = rowLabel(row)
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <Badge variant="outline" className="h-6 gap-0 pr-0.5 font-mono">
        <PopoverTrigger
          render={
            <button
              type="button"
              aria-label={`Edit filter ${label}`}
              className="h-full rounded-control px-1 outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
          }
        >
          {label}
        </PopoverTrigger>
        <button
          type="button"
          aria-label={`Remove filter ${label}`}
          className="inline-flex size-4 items-center justify-center rounded-control outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring"
          onClick={onRemove}
        >
          <XIcon aria-hidden="true" />
        </button>
      </Badge>
      <PopoverContent align="start" className="w-80">
        <RowForm
          initial={row}
          submitLabel="Update filter"
          onSubmit={(next) => {
            setOpen(false)
            onSave(next)
          }}
        />
      </PopoverContent>
    </Popover>
  )
}

function AddFilter({ onAdd }: { onAdd: (row: BuilderRow) => void }) {
  const [open, setOpen] = useState(false)
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        render={<Button type="button" variant="outline" size="xs" />}
      >
        <PlusIcon aria-hidden="true" />
        Add a filter
      </PopoverTrigger>
      <PopoverContent align="start" className="w-80">
        <RowForm
          initial={{ id: "new", field: "", op: "is", value: "", values: [] }}
          submitLabel="Add filter"
          onSubmit={(next) => {
            setOpen(false)
            onAdd(next)
          }}
        />
      </PopoverContent>
    </Popover>
  )
}

export function BuilderBar() {
  const { draft, setDraft, run, setMode, focusQuery } = useLogsWorkspace()
  const rows = rowsOf(draft) ?? []
  const text = textOf(rows)

  function commit(next: BuilderRow[]) {
    const nextText = textOf(next)
    setDraft(nextText)
    run(nextText)
  }

  return (
    <div className="flex flex-col gap-2">
      <div className="flex flex-wrap items-center gap-1.5">
        {rows.map((row) => (
          <FilterChip
            key={row.id}
            row={row}
            onSave={(next) =>
              commit(rows.map((item) => (item.id === row.id ? next : item)))
            }
            onRemove={() => commit(rows.filter((item) => item.id !== row.id))}
          />
        ))}
        <AddFilter
          onAdd={(row) =>
            commit([...rows, { ...row, id: String(rows.length) }])
          }
        />
      </div>
      <div className="flex flex-wrap items-center gap-2">
        <code className="min-w-0 truncate font-mono text-xs text-muted-foreground">
          {text === "" ? "No filters yet" : text}
        </code>
        <Button
          type="button"
          variant="link"
          size="xs"
          onClick={() => {
            setMode("code")
            focusQuery()
          }}
        >
          Edit as KQL
        </Button>
      </div>
    </div>
  )
}
