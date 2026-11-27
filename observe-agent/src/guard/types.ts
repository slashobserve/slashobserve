import type { CallMeta, GuardFailureCode, SettledKind } from "../protocol"

export type Mode = "create" | "revise" | "suggest" | "investigate"

export type ToolPhase =
  "discover" | "lint" | "query" | "save" | "write" | "ledger"

export type StepPhase = ToolPhase | "repair" | "stage"

export type Stage = "orient" | "hypothesize" | "test" | "conclude"

export type Verdict = "dispatch" | "replay" | "reuse" | "refuse"

export type Hypothesis = {
  id: string
  text: string
  status: string
  closing?: string
  knock_on_of?: string
  evidence?: string[]
  next?: string
}

export type Call = {
  name: string
  input: string
  ungrounded?: string
  unclosed?: string
}

export type Unchecked = { datasource: string; code: string }

export type Admission = {
  verdict: Verdict
  phase: StepPhase
  key: string
  position: number
  reuseOf?: number
  content?: string
  step?: string
  note?: string
  readsRefused?: number
  unchecked?: Unchecked[]
}

export type CallResult = {
  isError: boolean
  content: string
  settled: SettledKind | null
  call?: CallMeta
}

export type Stop = {
  failure: GuardFailureCode
  step: string
  message: string
}

export type Settled = { note?: string; stop?: Stop }

export type TurnEnd = {
  nudge: string
  stageLine: string
  stageStep: string
  notes: string[]
  stop?: Stop
}

export type GuardUsage = {
  nudges: number
  save_now_refusals: number
  stalled_turns: number
  one_panel_refusals: number
  repair_rounds: number
  call_limit?: number
  stage_at_save?: Stage
  hypotheses?: Hypothesis[]
}
