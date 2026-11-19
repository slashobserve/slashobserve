import { useMemo, type ReactNode } from "react"

import type { Selections } from "@/api/keys"
import type { Dashboard } from "@/spec"

import type { PanelItem } from "../dashboard-grid"
import { RowHeader } from "../rows/row-header"
import { printEntries } from "./print-layout"

export const sheetWidth = 680

type Props = {
  spec: Dashboard
  selections: Selections
  values: Record<string, string[] | undefined>
  renderPanel: (
    item: PanelItem,
    size: { width: number; height: number }
  ) => ReactNode
}

export function PrintGrid({ spec, selections, values, renderPanel }: Props) {
  const items = useMemo(
    () => printEntries(spec, selections, (name) => values[name]),
    [spec, selections, values]
  )
  return (
    <div className="flex flex-col gap-2.5">
      {items.map(({ entry, height }) => (
        <div
          key={entry.key}
          data-export-item={entry.kind}
          style={{ height, width: sheetWidth }}
          className={
            entry.kind === "row" ? "break-after-avoid" : "break-inside-avoid"
          }
        >
          {entry.kind === "row" ? (
            <RowHeader
              rowId={entry.row.id}
              title={entry.title}
              collapsed={entry.collapsed}
              childCount={entry.childCount}
              onToggle={undefined}
            />
          ) : null}
          {entry.kind === "note" ? (
            <p className="flex size-full items-center text-xs text-muted-foreground">
              {entry.text}
            </p>
          ) : null}
          {entry.kind === "panel"
            ? renderPanel(entry, { width: sheetWidth, height })
            : null}
        </div>
      ))}
    </div>
  )
}
