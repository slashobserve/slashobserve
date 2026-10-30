import { compileNamePattern } from "@/spec"
import type { Transform } from "@/spec"

import {
  copyFrame,
  seriesName,
  withFields,
  type PreparedField,
  type PreparedFrame,
} from "./names"

type RenameOptions = Extract<Transform, { type: "renameByRegex" }>

export function renameByRegex(
  frames: PreparedFrame[],
  { regex, renameTo }: RenameOptions
): PreparedFrame[] {
  const pattern = compileNamePattern(regex)
  if (pattern === null) return frames.map(copyFrame)
  return frames.map((frame) =>
    withFields(
      frame,
      frame.fields.map((field): PreparedField => {
        const display = [seriesName(frame, field), field.name].find((text) =>
          pattern.test(text)
        )
        if (display === undefined) return field
        const name = display.replace(pattern, renameTo)
        return { ...field, name, seriesName: name }
      })
    )
  )
}
