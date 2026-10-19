import type { WireFrame } from "@/dashboards/queries"
import { queryRef, type DataPanel } from "@/spec"

import { seriesLabel } from "../frames"
import { numberFieldCount, type PreparedFrame } from "./names"

export function stampLegends(
  panel: DataPanel,
  frames: WireFrame[]
): PreparedFrame[] {
  return frames.map((frame) => {
    const query = panel.queries.find(
      (candidate, index) => queryRef(candidate, index) === frame.ref
    )
    if (!query?.legend || query.format === "table") {
      return { ...frame, fields: [...frame.fields] }
    }
    const count = numberFieldCount(frame)
    return {
      ...frame,
      fields: frame.fields.map((field) => {
        if (field.type !== "number") return field
        const name = seriesLabel(frame, field, query.legend, count)
        return name ? { ...field, seriesName: name } : field
      }),
    }
  })
}
