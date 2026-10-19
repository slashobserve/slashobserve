import type { WireFrame } from "@/dashboards/queries"
import type { DataPanel, Transform } from "@/spec"

import { calculateField } from "./calculate"
import { filterFieldsByName } from "./filter"
import { applyQueryFormats } from "./format"
import { joinByField } from "./join"
import { limit } from "./limit"
import { merge } from "./merge"
import { organize } from "./organize"
import { reduceFrames } from "./reduce"
import { seriesToRows } from "./series-to-rows"
import { sortBy } from "./sort"

function applyTransform(
  frames: WireFrame[],
  transform: Transform
): WireFrame[] {
  switch (transform.type) {
    case "merge":
      return merge(frames)
    case "seriesToRows":
      return seriesToRows(frames)
    case "organize":
      return organize(frames, transform)
    case "reduce":
      return reduceFrames(frames, transform)
    case "joinByField":
      return joinByField(frames, transform)
    case "filterFieldsByName":
      return filterFieldsByName(frames, transform)
    case "calculateField":
      return calculateField(frames, transform)
    case "sortBy":
      return sortBy(frames, transform)
    case "limit":
      return limit(frames, transform)
  }
}

export function applyTransforms(
  frames: WireFrame[],
  transforms: readonly Transform[] | undefined
): WireFrame[] {
  return (transforms ?? []).reduce(applyTransform, frames)
}

export function prepareFrames(
  panel: DataPanel,
  frames: WireFrame[]
): WireFrame[] {
  return applyTransforms(applyQueryFormats(panel, frames), panel.transform)
}
