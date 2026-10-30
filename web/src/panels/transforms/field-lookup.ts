import type { Transform } from "@/spec"

import type { Gazetteers } from "../geomap/gazetteer/lookup"
import { locate, withLocationFields } from "./location"
import { copyFrame, type PreparedFrame } from "./names"

type FieldLookupOptions = Extract<Transform, { type: "fieldLookup" }>

export function fieldLookup(
  frames: PreparedFrame[],
  { lookupField, gazetteer }: FieldLookupOptions,
  gazetteers: Gazetteers
): PreparedFrame[] {
  return frames.map((frame) => {
    const places = locate(
      frame,
      { mode: "lookup", lookup: lookupField, gazetteer },
      gazetteers
    )
    return places ? withLocationFields(frame, places) : copyFrame(frame)
  })
}
