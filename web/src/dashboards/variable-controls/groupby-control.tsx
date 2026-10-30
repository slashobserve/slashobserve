import { useId, useState, type FormEvent } from "react"
import { PlusIcon, XIcon } from "lucide-react"
import { useQuery } from "@tanstack/react-query"

import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"
import { Input } from "@/components/ui/input"
import { labelNamesQuery } from "@/panel-editor/queries"
import type { GroupByVariable } from "@/spec"

import type { VariableState } from "../use-variables"
import { isGroupByKey, maxGroupByKeys } from "../variables"
import { SuggestInput } from "./suggest-input"
import { VariableBox, variableLabel } from "./variable-box"

type Props = {
  variable: GroupByVariable
  state: VariableState
  datasource: string | undefined
  datasourceType: string | undefined
  onChange: (values: string[]) => void
}

type FormProps = {
  datasource: string | undefined
  datasourceType: string | undefined
  selected: string[]
  onAdd: (key: string) => void
}

function GroupByKeyForm({
  datasource,
  datasourceType,
  selected,
  onAdd,
}: FormProps) {
  const keyId = useId()
  const [key, setKey] = useState("")
  const suggests = datasource !== undefined && datasourceType === "prometheus"
  const names = useQuery({
    ...labelNamesQuery(datasource ?? "", {}),
    enabled: suggests,
  })
  const suggestions = (names.data?.labels ?? []).filter(
    (label) => !selected.includes(label)
  )
  const full = selected.length >= maxGroupByKeys
  const problem = key !== "" && !isGroupByKey(key)
  const canAdd = !full && isGroupByKey(key) && !selected.includes(key)

  function submit(event: FormEvent) {
    event.preventDefault()
    if (canAdd) onAdd(key)
  }

  return (
    <form className="flex flex-col gap-2.5" onSubmit={submit}>
      <div className="flex flex-col gap-1.5">
        <Label htmlFor={keyId}>Label</Label>
        {suggests ? (
          <SuggestInput
            id={keyId}
            value={key}
            suggestions={suggestions}
            onChange={setKey}
          />
        ) : (
          <Input
            id={keyId}
            value={key}
            onChange={(event) => setKey(event.target.value)}
          />
        )}
        {problem ? (
          <p className="text-xs text-destructive">Use a label name.</p>
        ) : null}
      </div>
      {full ? (
        <p className="text-xs text-destructive">
          At most {maxGroupByKeys} labels.
        </p>
      ) : null}
      <div className="flex justify-end">
        <Button type="submit" size="sm" disabled={!canAdd}>
          Add
        </Button>
      </div>
    </form>
  )
}

export function GroupByControl({
  variable,
  state,
  datasource,
  datasourceType,
  onChange,
}: Props) {
  const id = useId()
  const [open, setOpen] = useState(false)
  const keys = state.selected.filter(isGroupByKey)

  function add(key: string) {
    if (!keys.includes(key)) onChange([...keys, key])
    setOpen(false)
  }

  return (
    <VariableBox variable={variable} id={id}>
      <div
        role="group"
        aria-label={variableLabel(variable)}
        className="flex min-w-0 flex-wrap items-center gap-1 px-1 py-0.5"
      >
        {keys.map((key) => (
          <span
            key={key}
            className="flex h-5 items-center gap-0.5 rounded-sm bg-muted pl-1.5 text-xs"
          >
            <span className="font-mono">{key}</span>
            <Button
              variant="ghost"
              size="icon-xs"
              className="size-5 opacity-60 hover:opacity-100"
              aria-label={`Remove ${key} from ${variableLabel(variable)}`}
              onClick={() => onChange(keys.filter((item) => item !== key))}
            >
              <XIcon />
            </Button>
          </span>
        ))}
        <Popover open={open} onOpenChange={setOpen}>
          <PopoverTrigger
            render={
              <Button
                variant="ghost"
                size="sm"
                className="h-5 rounded-control px-1.5 text-[13px] font-normal"
              />
            }
          >
            <PlusIcon />
            Add label
          </PopoverTrigger>
          <PopoverContent align="start" className="w-72">
            <GroupByKeyForm
              datasource={datasource}
              datasourceType={datasourceType}
              selected={keys}
              onAdd={add}
            />
          </PopoverContent>
        </Popover>
      </div>
    </VariableBox>
  )
}
