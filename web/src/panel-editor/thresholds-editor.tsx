import { useId, useState, type Dispatch } from "react"
import { XIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { sameJson } from "@/lib/json"
import { thresholdColor, thresholdIcons } from "@/panels/thresholds"
import type { Threshold } from "@/spec"

import type { DraftAction } from "./draft"
import { NativeSelect } from "./field-controls"
import {
  addRow,
  maxThresholds,
  rowsFromThresholds,
  setRowColor,
  thresholdColors,
  thresholdsFromRows,
  thresholdWords,
  type ThresholdColor,
  type ThresholdRow,
} from "./threshold-rows"

function rowColor(row: ThresholdRow): string {
  return row.color === "custom" ? row.hex : thresholdColor(row.color)
}

export function ThresholdsEditor({
  thresholds,
  dispatch,
  readOnly,
}: {
  thresholds: readonly Threshold[] | undefined
  dispatch: Dispatch<DraftAction>
  readOnly: boolean
}) {
  const headingId = useId()
  const [rows, setRows] = useState(() => rowsFromThresholds(thresholds))
  const [synced, setSynced] = useState(thresholds)
  if (!sameJson(thresholds, synced)) {
    setSynced(thresholds)
    setRows(rowsFromThresholds(thresholds))
  }

  const result = thresholdsFromRows(rows)

  const update = (next: ThresholdRow[]) => {
    setRows(next)
    const applied = thresholdsFromRows(next)
    if (!applied.ok) return
    setSynced(applied.thresholds ?? undefined)
    dispatch({ type: "setThresholds", thresholds: applied.thresholds })
  }

  const edit = (key: string, change: (row: ThresholdRow) => ThresholdRow) =>
    update(rows.map((row) => (row.key === key ? change(row) : row)))

  return (
    <div
      role="group"
      aria-labelledby={headingId}
      className="flex flex-col gap-1.5"
    >
      <span id={headingId} className="text-[13px] font-medium">
        Thresholds
      </span>
      {rows.map((row, index) => {
        const Icon = thresholdIcons[row.color]
        const word = thresholdWords[row.color]
        return (
          <div key={row.key} className="flex flex-wrap items-center gap-1.5">
            <Icon
              aria-hidden="true"
              className="size-3.5 shrink-0"
              style={{ color: rowColor(row) }}
            />
            <NativeSelect
              aria-label={`Color of threshold ${index + 1}`}
              value={row.color}
              disabled={readOnly}
              className="min-w-0 flex-1 px-2"
              onChange={(event) =>
                edit(row.key, (current) =>
                  setRowColor(current, event.target.value as ThresholdColor)
                )
              }
            >
              {thresholdColors.map((color) => (
                <option key={color} value={color}>
                  {thresholdWords[color]}
                </option>
              ))}
            </NativeSelect>
            <Input
              aria-label={`${word} from`}
              inputMode="decimal"
              value={row.value}
              placeholder={index === 0 ? "Base" : undefined}
              readOnly={readOnly}
              className="w-16 px-1.5 font-mono"
              onChange={(event) =>
                edit(row.key, (current) => ({
                  ...current,
                  value: event.target.value,
                }))
              }
            />
            <Button
              variant="ghost"
              size="icon-xs"
              aria-label={`Remove ${word} threshold`}
              disabled={readOnly}
              onClick={() =>
                update(rows.filter((item) => item.key !== row.key))
              }
            >
              <XIcon aria-hidden="true" />
            </Button>
            {row.color === "custom" ? (
              <div className="flex basis-full items-center gap-2 pl-[22px]">
                <span
                  aria-hidden="true"
                  className="size-3 shrink-0 rounded-sm border border-border"
                  style={{ backgroundColor: rowColor(row) }}
                />
                <Input
                  aria-label={`Hex color of threshold ${index + 1}`}
                  value={row.hex}
                  maxLength={7}
                  readOnly={readOnly}
                  className="w-[72px] px-1.5 font-mono"
                  onChange={(event) =>
                    edit(row.key, (current) => ({
                      ...current,
                      hex: event.target.value,
                    }))
                  }
                />
              </div>
            ) : null}
          </div>
        )
      })}
      {result.ok ? null : (
        <p role="alert" className="text-xs text-status-critical">
          {result.error}
        </p>
      )}
      <Button
        variant="link"
        size="xs"
        className="h-auto self-start px-0 text-xs font-normal text-muted-foreground underline"
        disabled={readOnly || rows.length >= maxThresholds}
        onClick={() => update(addRow(rows))}
      >
        Add threshold
      </Button>
    </div>
  )
}
