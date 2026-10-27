import { PuzzleIcon } from "lucide-react"

import { StatusLabel } from "@/components/status-label"
import { Cell, HeaderCell, TableCard } from "@/components/table-card"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"

import type { ImportSourceDatasource } from "./errors"
import type { Mapping } from "./queries"
import { datasourceKindLabel, visibleDatasources } from "./report"

type Props = {
  datasources: ImportSourceDatasource[]
  mapping: Mapping
  onPick: (key: string, uid: string) => void
  disabled: boolean
  busy: boolean
}

const headingId = "import-datasources"

function Choice({
  datasource,
  value,
  onPick,
  disabled,
}: {
  datasource: ImportSourceDatasource
  value: string | null
  onPick: (key: string, uid: string) => void
  disabled: boolean
}) {
  if (datasource.kind === null) {
    return (
      <span className="inline-flex items-center gap-1.5">
        <PuzzleIcon aria-hidden="true" className="size-3.5 flex-none" />
        Not supported yet, shown as placeholders
      </span>
    )
  }
  const kind = datasourceKindLabel(datasource.kind)
  if (datasource.candidates.length === 0) {
    return (
      <StatusLabel tone="critical">No {kind} datasource configured</StatusLabel>
    )
  }
  const items = datasource.candidates.map((uid) => ({
    value: uid,
    label: `${uid} (${kind})`,
  }))
  return (
    <Select
      items={items}
      value={value}
      disabled={disabled}
      onValueChange={(uid) => uid && onPick(datasource.key, uid)}
    >
      <SelectTrigger
        aria-label={`Datasource for ${datasource.key}`}
        className="w-full max-w-60"
      >
        <SelectValue placeholder="Choose a datasource" />
      </SelectTrigger>
      <SelectContent alignItemWithTrigger={false} align="start">
        {items.map((item) => (
          <SelectItem key={item.value} value={item.value}>
            {item.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

export function DatasourcesCard({
  datasources,
  mapping,
  onPick,
  disabled,
  busy,
}: Props) {
  return (
    <div
      aria-busy={busy || undefined}
      className="overflow-hidden rounded-card border bg-background"
    >
      <div className="border-b px-3.5 py-3">
        <h2 id={headingId} className="text-sm font-semibold">
          Datasources
        </h2>
        <p className="mt-0.5 text-xs text-muted-foreground">
          Match each Grafana datasource to one of yours.
        </p>
      </div>
      <TableCard
        labelledBy={headingId}
        minWidth={520}
        className="rounded-none border-0 focus-visible:ring-inset"
      >
        <thead>
          <tr>
            <HeaderCell>In Grafana</HeaderCell>
            <HeaderCell>Panels</HeaderCell>
            <HeaderCell>Use in SlashObserve</HeaderCell>
          </tr>
        </thead>
        <tbody>
          {visibleDatasources(datasources).map((datasource) => (
            <tr key={datasource.key}>
              <Cell>
                <span className="font-mono text-xs break-all">
                  {datasource.key}
                </span>
                <div className="text-xs text-muted-foreground">
                  {datasource.name ?? datasource.type}
                </div>
              </Cell>
              <Cell>{datasource.panels}</Cell>
              <Cell>
                <Choice
                  datasource={datasource}
                  value={mapping[datasource.key] ?? datasource.match}
                  onPick={onPick}
                  disabled={disabled}
                />
              </Cell>
            </tr>
          ))}
        </tbody>
      </TableCard>
    </div>
  )
}
