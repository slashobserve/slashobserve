export const signalRequests = "requests"
export const signalErrors = "errors"
export const signalLatency = "latency"

export const eventAnnotation = "annotation"
export const eventDeployLine = "deploy lines"
export const eventFlag = "flag"

export const second = 1000
export const minute = 60 * second
export const hour = 60 * minute

export const rungs = [hour, 6 * hour, 24 * hour] as const

export type Point = { at: number; value: number }

export type Row = {
  service: string
  signal: string
  rung: number
  window: number
  baseline: number
  unit: string
  source: string
  basis: string
  moved: boolean
  onset?: number | undefined
}

export type Origin = {
  source: string
  service: string
  operation: string
  message: string
  traces: number
  share: number
  example: string
  relayedBy: string[]
}

export type Event = {
  kind: string
  source: string
  service: string
  at: number
  text: string
}

export type Pattern = {
  source: string
  service: string
  level: string
  text: string
  window: number
  baseline: number
}

export type ServiceSource = {
  source: string
  type: string
  signal: string
  field: string
  services: string[]
}

export type Skip = {
  step: string
  source: string
  reason: string
  signals: string[]
}

export type Findings = {
  rows: Row[]
  origins: Origin[]
  events: Event[]
  patterns: Pattern[]
}

export type Candidate = {
  id: string
  service: string
  onset?: number | undefined
  rows: Row[]
  knockOn: string[]
  origins: Origin[]
  events: Event[]
  patterns: Pattern[]
}

export type Window = { from: number; to: number }

export type Report = {
  end: number
  question?: Window | undefined
  services: ServiceSource[]
  signals: string[]
  rates: Row[]
  candidates: Candidate[]
  background: Row[]
  skipped: Skip[]
  calls: number
  duration: number
}

export type Lead = { id: string; service: string; text: string; next: string }
