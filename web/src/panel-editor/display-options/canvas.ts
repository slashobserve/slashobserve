import type { DisplayOption } from "./types"
import { calculation } from "./shared"

export const canvas: readonly DisplayOption[] = [
  calculation,
  { kind: "note", text: "Edit elements and connections in the JSON view." },
]
