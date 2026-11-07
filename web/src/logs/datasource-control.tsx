import { useId, useState } from "react"
import { ChevronDownIcon } from "lucide-react"

import { StatusLabel } from "@/components/status-label"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Popover,
  PopoverContent,
  PopoverDescription,
  PopoverTitle,
  PopoverTrigger,
} from "@/components/ui/popover"
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group"
import { rememberDatasource } from "@/datasources/datasource-memory"
import type { DatasourceStatus } from "@/datasources/queries"

import { typeName } from "./datasource-picker"
import { isSearchFamily } from "./search"
import { useLogsWorkspace } from "./workspace"

const groupOrder = ["elasticsearch", "opensearch", "loki", "clickhouse"]

function languages(type: string): string {
  if (isSearchFamily(type)) return "KQL, Lucene or Query DSL"
  return type === "loki" ? "LogQL" : "SQL"
}

function groupsFor(
  datasources: readonly DatasourceStatus[],
  filter: string
): { type: string; rows: DatasourceStatus[] }[] {
  const needle = filter.trim().toLowerCase()
  const matching = datasources.filter(
    (datasource) =>
      needle === "" ||
      datasource.name.toLowerCase().includes(needle) ||
      datasource.uid.toLowerCase().includes(needle)
  )
  return groupOrder
    .map((type) => ({
      type,
      rows: matching.filter((datasource) => datasource.type === type),
    }))
    .filter((group) => group.rows.length > 0)
}

export function DatasourceControl() {
  const { datasource, datasources, search, update, announce } =
    useLogsWorkspace()
  const ids = useId()
  const [open, setOpen] = useState(false)
  const [picked, setPicked] = useState(datasource.uid)
  const [filter, setFilter] = useState("")
  const groups = groupsFor(datasources, filter)

  function onOpenChange(next: boolean) {
    if (next) {
      setPicked(datasource.uid)
      setFilter("")
    }
    setOpen(next)
  }

  function apply() {
    const target = datasources.find((candidate) => candidate.uid === picked)
    setOpen(false)
    if (target === undefined || target.uid === datasource.uid) return
    rememberDatasource("logs", target.uid)
    const keepsQuery =
      isSearchFamily(datasource.type) && isSearchFamily(target.type)
    update({
      ds: target.uid,
      q: keepsQuery ? search.q : undefined,
      ask: undefined,
      line: undefined,
      cols: undefined,
    })
    announce(
      `Switched to ${target.name}; the query was ${keepsQuery ? "kept" : "cleared"}`
    )
  }

  const where = [datasource.index, datasource.time_field]
  const placement = where.every((part) => part !== undefined) ? where : []

  return (
    <Popover open={open} onOpenChange={onOpenChange}>
      <PopoverTrigger
        render={
          <Button
            type="button"
            variant="outline"
            aria-haspopup="dialog"
            className="max-w-full"
          />
        }
      >
        <span className="truncate">
          {[datasource.name, typeName(datasource.type), ...placement].join(" ")}
        </span>
        <ChevronDownIcon aria-hidden="true" />
      </PopoverTrigger>
      <PopoverContent
        role="dialog"
        aria-label="Datasources"
        align="end"
        className="w-90 gap-3"
      >
        <div className="flex flex-col gap-0.5">
          <PopoverTitle>Datasources</PopoverTitle>
          <PopoverDescription>
            One datasource at a time, queried in its own language.
          </PopoverDescription>
        </div>
        <Input
          type="search"
          aria-label="Search datasources"
          placeholder="Search datasources"
          value={filter}
          onChange={(event) => setFilter(event.target.value)}
        />
        {groups.length === 0 ? (
          <p className="text-[13px] text-muted-foreground">
            No datasources match.
          </p>
        ) : (
          <RadioGroup
            aria-label="Datasource"
            value={picked}
            onValueChange={(next) => setPicked(next as string)}
            className="max-h-80 gap-3 overflow-y-auto"
          >
            {groups.map((group) => (
              <section
                key={group.type}
                aria-labelledby={`${ids}-${group.type}`}
                className="flex flex-col gap-1.5"
              >
                <h3
                  id={`${ids}-${group.type}`}
                  className="text-xs font-medium text-muted-foreground"
                >
                  {typeName(group.type)}
                </h3>
                {group.rows.map((row) => (
                  <label
                    key={row.uid}
                    className="flex items-start gap-2 text-[13px]"
                  >
                    <RadioGroupItem value={row.uid} className="mt-0.5" />
                    <span className="flex min-w-0 flex-1 flex-col">
                      <span className="flex flex-wrap items-center gap-x-2">
                        <span className="truncate">{row.name}</span>
                        <span className="font-mono text-xs text-muted-foreground">
                          {row.uid}
                        </span>
                        {row.healthy ? null : (
                          <StatusLabel tone="critical" title={row.message}>
                            unavailable
                          </StatusLabel>
                        )}
                      </span>
                      <span className="text-xs text-muted-foreground">
                        {languages(row.type)}
                      </span>
                    </span>
                  </label>
                ))}
              </section>
            ))}
          </RadioGroup>
        )}
        <p className="text-xs text-muted-foreground">
          Switching keeps the time range and filter pills; the query is
          rewritten in the new language where the builder can, otherwise
          cleared.
        </p>
        <div className="flex justify-end gap-2">
          <Button
            type="button"
            variant="outline"
            onClick={() => setOpen(false)}
          >
            Cancel
          </Button>
          <Button type="button" onClick={apply}>
            Apply
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  )
}
