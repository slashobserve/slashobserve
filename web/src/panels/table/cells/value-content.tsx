import { thresholdIcons } from "../../thresholds"
import type { Cell } from "../table-model"

export function ValueContent({ cell }: { cell: Cell }) {
  const Icon =
    cell.level && cell.level !== "base" ? thresholdIcons[cell.level] : null
  return (
    <>
      {Icon ? <Icon className="size-3 shrink-0" aria-hidden="true" /> : null}
      {cell.text}
      {cell.word ? <span className="sr-only">, {cell.word}</span> : null}
    </>
  )
}
