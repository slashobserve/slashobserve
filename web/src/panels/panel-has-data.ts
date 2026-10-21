import type { WireFrame } from "@/dashboards/queries"
import type { DataPanel } from "@/spec"

import type { FieldResolver } from "./field-config"
import { hasData } from "./frames"
import { prepareFrames } from "./transforms"

export function panelHasData(
  panel: DataPanel,
  frames: WireFrame[],
  fields: FieldResolver
): boolean {
  const prepared = prepareFrames(panel, frames)
  if (panel.type !== "timeseries") return hasData(prepared)
  return hasData(
    prepared.map((frame) => ({
      ...frame,
      fields: frame.fields.filter(
        (field) =>
          field.type === "time" || !fields.configFor(frame, field).hidden
      ),
    }))
  )
}
