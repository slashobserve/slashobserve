import { PuzzleIcon } from "lucide-react"

import { DataGrid, type DataGridColumn } from "@/components/data-grid/data-grid"
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

function importDatasourceId(datasource: ImportSourceDatasource): string {
  return datasource.key
}

type Props = {
  datasources: ImportSourceDatasource[]
  mapping: Mapping
  onPick: (key: string, uid: string) => void
  disabled: boolean
  busy: boolean
}

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
      <span className="inline-flex items-center gap-1.5 text-muted-foreground">
        <PuzzleIcon aria-hidden="true" className="size-3.5 flex-none" />
        No {kind} datasource configured, shown as placeholders
      </span>
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
  const columns: DataGridColumn<ImportSourceDatasource>[] = [
    {
      id: "grafana",
      header: "In Grafana",
      minWidth: 180,
      autoHeight: true,
      cell: (datasource) => (
        <div className="py-2">
          <span className="font-mono text-xs break-all">{datasource.key}</span>
          <div className="text-xs text-muted-foreground">
            {datasource.name ?? datasource.type}
          </div>
        </div>
      ),
    },
    {
      id: "panels",
      header: "Panels",
      width: 100,
      cell: (datasource) => datasource.panels,
    },
    {
      id: "use",
      header: "Use in SlashObserve",
      minWidth: 240,
      flex: 2,
      cell: (datasource) => (
        <Choice
          datasource={datasource}
          value={mapping[datasource.key] ?? datasource.match}
          onPick={onPick}
          disabled={disabled}
        />
      ),
    },
  ]

  return (
    <div
      aria-busy={busy || undefined}
      className="overflow-hidden rounded-card border bg-background"
    >
      <div className="border-b px-3.5 py-3">
        <h2 className="text-sm font-semibold">Datasources</h2>
        <p className="mt-0.5 text-xs text-muted-foreground">
          Match each Grafana datasource to one of yours.
        </p>
      </div>
      <DataGrid
        label="Datasources"
        rows={visibleDatasources(datasources)}
        columns={columns}
        rowId={importDatasourceId}
        bare
      />
    </div>
  )
}
