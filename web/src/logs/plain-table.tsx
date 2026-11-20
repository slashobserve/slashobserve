import type { ReactNode } from "react"

import { Cell, HeaderCell, TableCard } from "@/components/table-card"
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

export function PlainTable({
  frame,
  note = untimedNote,
  renderCell,
}: {
  frame: WireFrame
  note?: string
  renderCell?: (field: string, value: unknown, type: string) => ReactNode
}) {
  const rows = frame.fields.reduce(
    (most, field) => Math.max(most, field.values.length),
    0
  )
  return (
    <div className="flex min-h-0 flex-1 flex-col gap-2 p-3">
      <p className="text-xs text-muted-foreground">{note}</p>
      <TableCard label="Query results" minWidth={480}>
        <thead>
          <tr>
            {frame.fields.map((field) => (
              <HeaderCell key={field.name}>{field.name}</HeaderCell>
            ))}
          </tr>
        </thead>
        <tbody>
          {Array.from({ length: rows }, (_, row) => (
            <tr key={row}>
              {frame.fields.map((field) => (
                <Cell key={field.name} className="font-mono">
                  {renderCell?.(field.name, field.values[row], field.type) ??
                    cellText(field.values[row])}
                </Cell>
              ))}
            </tr>
          ))}
        </tbody>
      </TableCard>
    </div>
  )
}
