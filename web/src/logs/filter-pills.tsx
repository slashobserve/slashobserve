import { useEffect, useMemo, useRef, useState } from "react"
import { useQuery } from "@tanstack/react-query"
import { PinIcon } from "lucide-react"

import { StatusLabel } from "@/components/status-label"
import { Button } from "@/components/ui/button"
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"
import { cn } from "@/lib/utils"
import { labelNamesQuery } from "@/panel-editor/queries"

import { typeName } from "./datasource-picker"
import { FilterForm } from "./filter-form"
import {
  addPill,
  applyPills,
  decodePills,
  encodePills,
  pillLabel,
  pillSupported,
  removePill,
  replacePill,
  unpinnedCleared,
  type NewPill,
  type Pill,
} from "./pills"
import { queryModeLabel } from "./search"
import { useLogsWorkspace } from "./workspace"

const pillClass =
  "inline-flex h-[26px] items-center gap-1 rounded-full border border-border px-2.5 text-xs outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring data-[negate=true]:border-status-critical data-[paused=true]:line-through data-[paused=true]:opacity-55"

function filtersClearedText(count: number): string {
  const noun = count === 1 ? "filter" : "filters"
  return `${count} ${noun} cleared; pinned filters kept`
}

function usePillStore() {
  const { search, update, run, announce } = useLogsWorkspace()
  const pills = useMemo(() => decodePills(search.f), [search.f])
  const q = search.q
  const filters = search.f

  function write(next: Pill[]) {
    update({ f: encodePills(next) })
    if (q !== undefined) run(q)
  }

  const previousQuery = useRef(q)
  const previousFilters = useRef(filters)
  useEffect(() => {
    const before = previousQuery.current
    const filtersBefore = previousFilters.current
    previousQuery.current = q
    previousFilters.current = filters
    if (before === undefined || before === q || filtersBefore !== filters) {
      return
    }
    const kept = unpinnedCleared(pills)
    if (kept.length === pills.length) return
    update({ f: encodePills(kept) }, { replace: true })
    announce(filtersClearedText(pills.length - kept.length))
  }, [q, filters, pills, update, announce])

  return { pills, write }
}

function PillMenuButton({
  onClick,
  children,
}: {
  onClick: () => void
  children: string
}) {
  return (
    <Button
      type="button"
      variant="ghost"
      size="sm"
      className="justify-start"
      onClick={onClick}
    >
      {children}
    </Button>
  )
}

function PillChip({
  pill,
  fields,
  supported,
  onChange,
  onReplace,
  onDelete,
}: {
  pill: Pill
  fields: readonly string[]
  supported: boolean
  onChange: (next: Pill) => void
  onReplace: (next: NewPill[]) => void
  onDelete: () => void
}) {
  const { datasource } = useLogsWorkspace()
  const [open, setOpen] = useState(false)
  const [editing, setEditing] = useState(false)
  const label = pillLabel(pill)

  function act(next: Pill) {
    setOpen(false)
    onChange(next)
  }

  return (
    <li className="flex items-center gap-1.5">
      <Popover
        open={open}
        onOpenChange={(next) => {
          setOpen(next)
          if (!next) setEditing(false)
        }}
      >
        <PopoverTrigger
          render={
            <button
              type="button"
              aria-label={`Filter ${label}`}
              data-negate={pill.negate}
              data-paused={pill.paused}
              data-pinned={pill.pinned}
              className={cn(pillClass, "font-mono")}
            />
          }
        >
          {pill.pinned ? (
            <PinIcon aria-hidden="true" className="size-3" />
          ) : null}
          {label}
        </PopoverTrigger>
        <PopoverContent align="start" className={editing ? "w-80" : "w-40"}>
          {editing ? (
            <FilterForm
              initial={pill}
              fields={fields}
              onCancel={() => setEditing(false)}
              onSubmit={(next) => {
                setOpen(false)
                setEditing(false)
                onReplace(next)
              }}
            />
          ) : (
            <div role="menu" className="flex flex-col">
              {pill.kind === "range" ? null : (
                <PillMenuButton
                  onClick={() => act({ ...pill, negate: !pill.negate })}
                >
                  {pill.negate ? "Include" : "Exclude"}
                </PillMenuButton>
              )}
              <PillMenuButton
                onClick={() => act({ ...pill, pinned: !pill.pinned })}
              >
                {pill.pinned ? "Unpin" : "Pin"}
              </PillMenuButton>
              <PillMenuButton
                onClick={() => act({ ...pill, paused: !pill.paused })}
              >
                {pill.paused ? "Resume" : "Pause"}
              </PillMenuButton>
              <PillMenuButton onClick={() => setEditing(true)}>
                Edit
              </PillMenuButton>
              <PillMenuButton
                onClick={() => {
                  setOpen(false)
                  onDelete()
                }}
              >
                Delete
              </PillMenuButton>
            </div>
          )}
        </PopoverContent>
      </Popover>
      {supported ? null : (
        <StatusLabel tone="warning">
          not applied on {typeName(datasource.type)}
        </StatusLabel>
      )}
    </li>
  )
}

function AddFilter({
  fields,
  onAdd,
}: {
  fields: readonly string[]
  onAdd: (next: NewPill[]) => void
}) {
  const [open, setOpen] = useState(false)
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        render={<Button type="button" variant="ghost" size="sm" />}
      >
        Add filter
      </PopoverTrigger>
      <PopoverContent align="start" className="w-80">
        <FilterForm
          fields={fields}
          onCancel={() => setOpen(false)}
          onSubmit={(next) => {
            setOpen(false)
            onAdd(next)
          }}
        />
      </PopoverContent>
    </Popover>
  )
}

export function FilterPills() {
  const { datasource, search, range } = useLogsWorkspace()
  const { pills, write } = usePillStore()
  const type = datasource.type
  const names = useQuery(
    labelNamesQuery(datasource.uid, { from: range.from, to: range.to })
  )
  const fields = names.data?.labels ?? []
  const blocked =
    pills.some((pill) => !pill.paused) &&
    search.q !== undefined &&
    applyPills(search.q, type, pills) === null

  return (
    <div className="flex flex-col gap-1">
      <div className="flex flex-wrap items-center gap-1.5">
        <ul role="list" aria-label="Filters" className="contents">
          {pills.map((pill) => (
            <PillChip
              key={pill.id}
              pill={pill}
              fields={fields}
              supported={pillSupported(type, pill)}
              onChange={(next) =>
                write(pills.map((item) => (item.id === pill.id ? next : item)))
              }
              onReplace={(next) => write(replacePill(pills, pill.id, next))}
              onDelete={() => write(removePill(pills, pill.id))}
            />
          ))}
        </ul>
        <AddFilter
          fields={fields}
          onAdd={(next) =>
            write(next.reduce((current, item) => addPill(current, item), pills))
          }
        />
      </div>
      {pills.length > 0 ? (
        <p className="text-xs text-muted-foreground">
          Filters are added to the query in {queryModeLabel(type)}. Pinned ones
          stay when the query changes.
        </p>
      ) : null}
      {blocked ? (
        <StatusLabel tone="warning">
          Filters are not applied: switch the query to KQL.
        </StatusLabel>
      ) : null}
    </div>
  )
}
