import type { Transform } from "@/spec"

import { copyFrame, type PreparedFrame } from "./names"

type FilterRefOptions = Extract<Transform, { type: "filterByRefId" }>

export function filterByRefId(
  frames: PreparedFrame[],
  { include }: FilterRefOptions
): PreparedFrame[] {
  return frames.filter((frame) => include.includes(frame.ref)).map(copyFrame)
}
