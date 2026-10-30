import type { Cell } from "../table-model"
import type { CellSpec } from "./cell-model"
import { ValueContent } from "./value-content"

export function ValueText({ cell, spec }: { cell: Cell; spec: CellSpec }) {
  if (spec.valueMode === "hidden") {
    return (
      <span className="sr-only">
        <ValueContent cell={cell} />
      </span>
    )
  }
  return (
    <span
      className="inline-flex shrink-0 items-center gap-1 tabular-nums"
      style={
        spec.valueMode === "color" && cell.color
          ? { color: cell.color }
          : undefined
      }
    >
      <ValueContent cell={cell} />
    </span>
  )
}
