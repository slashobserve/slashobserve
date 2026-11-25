import { useMemo, useRef, useState } from "react"
import { DownloadIcon, ImageIcon } from "lucide-react"
import { toast } from "sonner"

import { DataGrid, type DataGridColumn } from "@/components/data-grid/data-grid"
import { useWidth } from "@/components/data-grid/use-width"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Label } from "@/components/ui/label"
import { createFieldResolver } from "@/panels/field-config"

import { downloadCsv, formattedCell, framesToTable } from "./csv"
import { dataColumnWidth, fitColumnWidths } from "./data-columns"
import type { InspectTarget } from "./inspect-pane"
import { savePanelPng } from "./png"

const shownRows = 500

type DataRow = { id: string; values: readonly string[] }

const dataRowId = (row: DataRow) => row.id

export function WaitingForData() {
  return (
    <p className="text-[13px] text-muted-foreground">
      Waiting for the panel&apos;s data. Scroll the panel into view if it has
      not loaded.
    </p>
  )
}

export function DataTab({
  target,
  title,
  zone,
}: {
  target: InspectTarget | null
  title: string
  zone: string | undefined
}) {
  const [formatted, setFormatted] = useState(true)
  const frames = target?.frames
  const panel = target?.panel
  const selections = target?.selections
  const format = useMemo(
    () =>
      formatted && panel
        ? formattedCell(createFieldResolver(panel, selections), zone)
        : undefined,
    [formatted, panel, selections, zone]
  )
  const table = useMemo(
    () => (frames ? framesToTable(frames, format) : undefined),
    [frames, format]
  )
  const rows = useMemo(
    () =>
      (table?.rows ?? [])
        .slice(0, shownRows)
        .map((values, index): DataRow => ({ id: String(index), values })),
    [table]
  )
  const gridRef = useRef<HTMLDivElement>(null)
  const available = useWidth(gridRef, (table?.columns.length ?? 0) > 0)
  const columns = useMemo(() => {
    const headers = table?.columns ?? []
    const widths = fitColumnWidths(
      headers.map((header, index) =>
        dataColumnWidth(header, table?.rows ?? [], index)
      ),
      available
    )
    return headers.map((header, index): DataGridColumn<DataRow> => ({
      id: String(index),
      header,
      minWidth: widths[index],
      cell: (row) => row.values[index],
    }))
  }, [table, available])
  const [saving, setSaving] = useState(false)
  if (!target || !frames || !table) return <WaitingForData />
  const body = target.body
  const savePng = () => {
    const element = body.current
    if (!element) return
    setSaving(true)
    savePanelPng(element, title)
      .catch(() => toast.error("Could not save the panel as PNG"))
      .finally(() => setSaving(false))
  }
  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <Label className="gap-2 text-[13px] font-medium">
          <Checkbox checked={formatted} onCheckedChange={setFormatted} />
          Formatted data
        </Label>
        <div className="flex flex-wrap items-center gap-1.5">
          <Button
            variant="outline"
            size="sm"
            onClick={() => downloadCsv(frames, title)}
          >
            <DownloadIcon aria-hidden="true" />
            Download CSV
          </Button>
          <Button
            variant="outline"
            size="sm"
            disabled={saving}
            onClick={savePng}
          >
            <ImageIcon aria-hidden="true" />
            {saving ? "Saving PNG..." : "Save as PNG"}
          </Button>
        </div>
      </div>
      <p className="text-xs text-muted-foreground">
        {table.rows.length > shownRows
          ? `The first ${shownRows} of ${table.rows.length} rows, after the panel's transforms. The CSV has every row.`
          : `${table.rows.length === 1 ? "1 row" : `${table.rows.length} rows`}, after the panel's transforms.`}
      </p>
      {table.columns.length > 0 ? (
        <div ref={gridRef}>
          <DataGrid
            label={`${title} data`}
            rows={rows}
            columns={columns}
            rowId={dataRowId}
            compact
            className="tabular-nums [&_.ag-styled-root]:[--ag-scrollbar-thumb-color:var(--muted-foreground)]!"
          />
        </div>
      ) : null}
    </div>
  )
}
