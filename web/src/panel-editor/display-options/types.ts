import type { Option } from "../field-controls"

export type NumberRule = {
  min: number
  max: number
  minExclusive?: boolean
  integer?: boolean
}

export type DisplayOption =
  | {
      kind: "choice"
      key: string
      label: string
      options: readonly Option[]
      absent?: string
      numeric?: boolean
    }
  | { kind: "toggle"; key: string; label: string }
  | { kind: "reducer"; key: string; label: string }
  | {
      kind: "reducers"
      key: string
      label: string
      options: readonly Option[]
      max?: number
    }
  | {
      kind: "number"
      key: string
      label: string
      hint: string
      rule: NumberRule
    }
  | {
      kind: "text"
      key: string
      label: string
      placeholder: string
      maxLength: number
    }
  | {
      kind: "buckets"
      key: string
      label: string
      sizeHint: string
      countHint: string
    }
  | { kind: "tags"; key: string; label: string; max: number }
  | {
      kind: "names"
      key: string
      label: string
      max: number
      placeholder?: string
    }
  | { kind: "note"; text: string }
