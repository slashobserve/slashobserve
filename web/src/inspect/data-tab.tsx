import { useMemo, useState } from "react"
import { DownloadIcon, ImageIcon } from "lucide-react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"

import { downloadCsv, framesToTable } from "./csv"
import type { InspectTarget } from "./inspect-pane"
import { savePanelPng } from "./png"

const shownRows = 500

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
}: {
  target: InspectTarget | null
  title: string
}) {
  const frames = target?.frames
  const table = useMemo(
    () => (frames ? framesToTable(frames) : undefined),
    [frames]
  )
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
      <div className="flex flex-wrap items-center gap-2">
        <Button
          variant="outline"
          size="sm"
          onClick={() => downloadCsv(frames, title)}
        >
          <DownloadIcon aria-hidden="true" />
          Download CSV
        </Button>
        <Button variant="outline" size="sm" disabled={saving} onClick={savePng}>
          <ImageIcon aria-hidden="true" />
          {saving ? "Saving PNG..." : "Save as PNG"}
        </Button>
      </div>
      <p className="text-xs text-muted-foreground">
        {table.rows.length > shownRows
          ? `The first ${shownRows} of ${table.rows.length} rows, after the panel's transforms. The CSV has every row.`
          : `${table.rows.length === 1 ? "1 row" : `${table.rows.length} rows`}, after the panel's transforms.`}
      </p>
      {table.columns.length > 0 ? (
        <div className="rounded-card border">
          <Table className="text-xs tabular-nums">
            <TableHeader>
              <TableRow>
                {table.columns.map((column, index) => (
                  <TableHead key={index} className="h-8 px-2">
                    {column}
                  </TableHead>
                ))}
              </TableRow>
            </TableHeader>
            <TableBody>
              {table.rows.slice(0, shownRows).map((row, index) => (
                <TableRow key={index}>
                  {row.map((value, column) => (
                    <TableCell key={column} className="px-2 py-1">
                      {value}
                    </TableCell>
                  ))}
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      ) : null}
    </div>
  )
}
