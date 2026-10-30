import { cn } from "@/lib/utils"

import { lcdCellCount, lcdLitCells } from "../../bargauge/bargauge-model"
import type { Cell } from "../table-model"
import type { CellSpec } from "./cell-model"
import { ValueText } from "./value-text"

const lcdExtent = 120

export function GaugeCell({
  cell,
  spec,
  gauge,
}: {
  cell: Cell
  spec: CellSpec
  gauge: { fraction: number; min: number; max: number; value: number | null }
}) {
  const color = cell.color ?? "var(--series-1)"
  const cells = lcdCellCount(lcdExtent)
  const lit = lcdLitCells(gauge.fraction, cells)
  const now = Math.min(gauge.max, Math.max(gauge.min, gauge.value ?? gauge.min))
  return (
    <span className="flex items-center gap-2">
      <span
        role="meter"
        aria-label={cell.text}
        aria-valuenow={now}
        aria-valuemin={gauge.min}
        aria-valuemax={gauge.max}
        aria-valuetext={cell.word ? `${cell.text}, ${cell.word}` : cell.text}
        data-threshold={cell.level ?? undefined}
        className={cn(
          "flex h-2 min-w-20 flex-1 overflow-hidden rounded-[2px]",
          spec.gaugeStyle === "basic" && "bg-muted"
        )}
      >
        {spec.gaugeStyle === "lcd" ? (
          <span className="flex size-full gap-[2px]">
            {Array.from({ length: cells }, (_, index) => (
              <span
                key={index}
                data-part="cell"
                data-lit={index < lit}
                className={cn("flex-1", index >= lit && "bg-muted")}
                style={index < lit ? { backgroundColor: color } : undefined}
              />
            ))}
          </span>
        ) : (
          <span
            data-part="fill"
            className="block h-full"
            style={{
              width: `${gauge.fraction * 100}%`,
              backgroundColor: color,
            }}
          />
        )}
      </span>
      <ValueText cell={cell} spec={spec} />
    </span>
  )
}
