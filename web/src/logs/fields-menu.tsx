import { useMemo, useState } from "react"
import {
  ArrowDownIcon,
  ArrowUpIcon,
  ChevronDownIcon,
  Columns3Icon,
} from "lucide-react"

import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import {
  Popover,
  PopoverContent,
  PopoverTitle,
  PopoverTrigger,
} from "@/components/ui/popover"
import { columnTemplate, type ColumnSort } from "@/panels/logs/logs-panel"

import { maxColumns, moveColumn } from "./line-fields"

const searchAfter = 12

export function FieldsMenu({
  names,
  chosen,
  onChange,
}: {
  names: readonly string[]
  chosen: readonly string[]
  onChange: (names: string[]) => void
}) {
  const [filter, setFilter] = useState("")
  const listed = useMemo(() => {
    const needle = filter.trim().toLowerCase()
    return [...names, ...chosen.filter((name) => !names.includes(name))].filter(
      (name) => name.toLowerCase().includes(needle)
    )
  }, [names, chosen, filter])
  const full = chosen.length >= maxColumns

  return (
    <Popover>
      <PopoverTrigger
        render={
          <Button
            type="button"
            variant="outline"
            size="xs"
            aria-haspopup="dialog"
          />
        }
      >
        <Columns3Icon aria-hidden="true" />
        Columns
        {chosen.length > 0 ? (
          <span className="text-muted-foreground tabular-nums">
            {chosen.length}
          </span>
        ) : null}
      </PopoverTrigger>
      <PopoverContent
        role="dialog"
        aria-label="Columns"
        align="end"
        className="w-72 gap-2"
      >
        <PopoverTitle>Columns</PopoverTitle>
        {names.length > searchAfter ? (
          <Input
            type="search"
            aria-label="Search fields"
            placeholder="Search fields"
            value={filter}
            onChange={(event) => setFilter(event.target.value)}
          />
        ) : null}
        {listed.length === 0 ? (
          <p className="text-[13px] text-muted-foreground">
            No fields in these lines.
          </p>
        ) : (
          <ul
            aria-label="Fields"
            className="flex max-h-64 flex-col gap-1 overflow-auto text-[13px]"
          >
            {listed.map((name) => {
              const checked = chosen.includes(name)
              return (
                <li key={name}>
                  <label className="flex items-center gap-2 font-mono text-xs break-all">
                    <Checkbox
                      checked={checked}
                      disabled={!checked && full}
                      onCheckedChange={(next) =>
                        onChange(
                          next
                            ? [...chosen, name]
                            : chosen.filter((item) => item !== name)
                        )
                      }
                    />
                    {name}
                  </label>
                </li>
              )
            })}
          </ul>
        )}
        <Button
          type="button"
          variant="outline"
          size="xs"
          className="self-start"
          disabled={chosen.length === 0}
          onClick={() => onChange([])}
        >
          Reset columns
        </Button>
      </PopoverContent>
    </Popover>
  )
}

function SortMark({ direction }: { direction: "asc" | "desc" }) {
  const Icon = direction === "asc" ? ArrowUpIcon : ArrowDownIcon
  return <Icon aria-hidden="true" className="size-3" />
}

export function ColumnsHeader({
  columns,
  levelColumn,
  order,
  sort,
  onFlipOrder,
  onSort,
  onColumnsChange,
}: {
  columns: readonly string[]
  levelColumn: boolean
  order: "newest" | "oldest"
  sort: ColumnSort | null
  onFlipOrder: () => void
  onSort: (sort: ColumnSort) => void
  onColumnsChange: (names: string[]) => void
}) {
  return (
    <div
      aria-label="Column headers"
      role="group"
      className="grid shrink-0 [scrollbar-gutter:stable] items-center gap-x-2 overflow-hidden border-y border-border px-2 text-[11px] text-muted-foreground"
      style={{
        gridTemplateColumns: columnTemplate({
          showTime: true,
          levelColumn,
          count: columns.length,
        }),
      }}
    >
      <button
        type="button"
        aria-label={`Time, ${order === "newest" ? "newest" : "oldest"} first. Flip the sort`}
        onClick={onFlipOrder}
        className="flex h-6 items-center gap-1 rounded-control text-left outline-none focus-visible:ring-2 focus-visible:ring-ring"
      >
        Time
        {sort === null ? (
          <SortMark direction={order === "newest" ? "desc" : "asc"} />
        ) : null}
      </button>
      {levelColumn ? <span aria-hidden="true" /> : null}
      {columns.map((name, index) => (
        <DropdownMenu key={name}>
          <DropdownMenuTrigger
            aria-label={`${name} column menu`}
            className="flex h-6 min-w-0 items-center gap-1 rounded-control text-left font-mono outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            <span className="truncate">{name}</span>
            {sort?.name === name ? (
              <SortMark direction={sort.direction} />
            ) : (
              <ChevronDownIcon aria-hidden="true" className="size-3 shrink-0" />
            )}
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start">
            <DropdownMenuItem
              onClick={() => onSort({ name, direction: "asc" })}
            >
              Sort A to Z
            </DropdownMenuItem>
            <DropdownMenuItem
              onClick={() => onSort({ name, direction: "desc" })}
            >
              Sort Z to A
            </DropdownMenuItem>
            <DropdownMenuItem
              disabled={index === 0}
              onClick={() => onColumnsChange(moveColumn(columns, name, -1))}
            >
              Move left
            </DropdownMenuItem>
            <DropdownMenuItem
              disabled={index === columns.length - 1}
              onClick={() => onColumnsChange(moveColumn(columns, name, 1))}
            >
              Move right
            </DropdownMenuItem>
            <DropdownMenuItem
              variant="destructive"
              onClick={() =>
                onColumnsChange(columns.filter((item) => item !== name))
              }
            >
              Remove column
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      ))}
      <span className="h-6 leading-6">Message</span>
    </div>
  )
}
