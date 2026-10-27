import { useId, useState, type FormEvent } from "react"
import { useQuery } from "@tanstack/react-query"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { labelNamesQuery, labelValuesQuery } from "@/panel-editor/queries"

import {
  adhocOps,
  keyHint,
  maxAdhocFilters,
  validFilter,
  validKey,
  type AdhocFilter,
  type AdhocOp,
} from "./adhoc"
import { SuggestInput } from "./suggest-input"

type FieldProps = {
  id: string
  value: string
  suggestions: string[] | undefined
  onChange: (value: string) => void
}

function NameField({ id, value, suggestions, onChange }: FieldProps) {
  if (suggestions === undefined) {
    return (
      <Input
        id={id}
        value={value}
        onChange={(event) => onChange(event.target.value)}
      />
    )
  }
  return (
    <SuggestInput
      id={id}
      value={value}
      suggestions={suggestions}
      onChange={onChange}
    />
  )
}

type Props = {
  datasource: string
  datasourceType: string | undefined
  count: number
  onAdd: (filter: AdhocFilter) => void
}

export function AdhocFilterForm({
  datasource,
  datasourceType,
  count,
  onAdd,
}: Props) {
  const keyId = useId()
  const opId = useId()
  const valueId = useId()
  const [key, setKey] = useState("")
  const [op, setOp] = useState<AdhocOp>("=")
  const [value, setValue] = useState("")

  const suggests = datasourceType === "prometheus"
  const names = useQuery({
    ...labelNamesQuery(datasource, {}),
    enabled: suggests,
  })
  const keyKnown = suggests && (names.data?.labels.includes(key) ?? false)
  const values = useQuery({
    ...labelValuesQuery(datasource, key, {}),
    enabled: keyKnown,
  })

  const full = count >= maxAdhocFilters
  const keyProblem = key !== "" && !validKey(key, datasourceType)
  const filter: AdhocFilter = { key, op, value }
  const canAdd = !full && value !== "" && validFilter(filter, datasourceType)

  function submit(event: FormEvent) {
    event.preventDefault()
    if (canAdd) onAdd(filter)
  }

  return (
    <form className="flex flex-col gap-2.5" onSubmit={submit}>
      <div className="flex flex-col gap-1.5">
        <Label htmlFor={keyId}>Key</Label>
        <NameField
          id={keyId}
          value={key}
          suggestions={suggests ? (names.data?.labels ?? []) : undefined}
          onChange={setKey}
        />
        {keyProblem ? (
          <p className="text-xs text-destructive">{keyHint(datasourceType)}</p>
        ) : null}
      </div>
      <div className="flex flex-col gap-1.5">
        <Label htmlFor={opId}>Operator</Label>
        <Select
          items={adhocOps.map((item) => ({ value: item, label: item }))}
          value={op}
          onValueChange={(next) => {
            if (next !== null) setOp(next)
          }}
        >
          <SelectTrigger id={opId} className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent alignItemWithTrigger={false} align="start">
            {adhocOps.map((item) => (
              <SelectItem key={item} value={item}>
                {item}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <div className="flex flex-col gap-1.5">
        <Label htmlFor={valueId}>Value</Label>
        <NameField
          id={valueId}
          value={value}
          suggestions={suggests ? (values.data?.values ?? []) : undefined}
          onChange={setValue}
        />
      </div>
      {full ? (
        <p className="text-xs text-destructive">At most 20 filters.</p>
      ) : null}
      <div className="flex justify-end">
        <Button type="submit" size="sm" disabled={!canAdd}>
          Add
        </Button>
      </div>
    </form>
  )
}
