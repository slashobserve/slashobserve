import { useMemo, type ReactNode } from "react"

import { DataGrid, type DataGridColumn } from "@/components/data-grid/data-grid"
import type { PanelDataResponse, WireFrame } from "@/dashboards/queries"

export function isPlainResult(data: PanelDataResponse): boolean {
  return (
    data.meta.page?.plain === true ||
    !data.frames.some((frame) =>
      frame.fields.some((field) => field.type === "time")
    )
  )
}

export function cellText(value: unknown): string {
  if (value === null || value === undefined) return ""
  return typeof value === "object" ? JSON.stringify(value) : String(value)
}

const untimedNote =
  "This query selects no Timestamp, so the rows show as a table."

const rowId = (row: number) => String(row)

export function PlainTable({
  frame,
  note = untimedNote,
  renderCell,
}: {
  frame: WireFrame
  note?: string
  renderCell?: (field: string, value: unknown, type: string) => ReactNode
}) {
  const rows = useMemo(
    () =>
      Array.from(
        {
          length: frame.fields.reduce(
            (most, field) => Math.max(most, field.values.length),
            0
          ),
        },
        (_, row) => row
      ),
    [frame]
  )
  const columns = useMemo<DataGridColumn<number>[]>(
    () =>
      frame.fields.map((field) => ({
        id: field.name,
        header: field.name,
        minWidth: 120,
        autoHeight: true,
        cell: (row) => {
          const value = field.values[row]
          const content =
            renderCell?.(field.name, value, field.type) ?? cellText(value)
          return content === "" ? null : (
            <span className="min-w-0 py-2 font-mono break-words whitespace-normal">
              {content}
            </span>
          )
        },
      })),
    [frame, renderCell]
  )
  return (
    <div className="flex min-h-0 flex-1 flex-col gap-2 p-3">
      <p className="text-xs text-muted-foreground">{note}</p>
      <DataGrid
        label="Query results"
        rows={rows}
        columns={columns}
        rowId={rowId}
      />
    </div>
  )
}
