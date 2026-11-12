import { z } from "zod"

export const turnMetaKey = "slashobserve.com/turn"
export const runMetaKey = "slashobserve.com/run"

export const defaultSessionRenewBytes = 256 << 10

export const loopFailureCodes = [
  "call_timeout",
  "rate_limited",
  "provider_error",
  "output_limit",
  "no_tool_call",
  "no_finding",
  "refused",
  "no_fixture",
  "internal",
] as const
export type FailureCode = (typeof loopFailureCodes)[number]

export const briefReasons = [
  "restart",
  "output_limit",
  "pending",
  "conflict",
  "continue",
  "renewed",
  "call_timeout",
  "no_finding",
  "no_answer",
] as const
export type BriefReason = (typeof briefReasons)[number]

const count = z.number().int().nonnegative()

export const buildProgress = z.object({
  saves: count,
  panels: count,
  planned_panels: count,
})
export type BuildProgress = z.infer<typeof buildProgress>

export const turnMeta = z.strictObject({
  id: z.string().min(1),
  calls: z.number().int().min(1),
  saves: count,
})
export type TurnMeta = z.infer<typeof turnMeta>

export const settledKinds = [
  "saved",
  "finding",
  "suggested",
  "restarted",
  "ended",
] as const
export type SettledKind = (typeof settledKinds)[number]

export const runMeta = z.object({
  settled: z.enum(settledKinds).nullable(),
  build: buildProgress.nullable(),
})
export type RunMeta = z.infer<typeof runMeta>

export const noRunMeta: RunMeta = { settled: null, build: null }

export const modelSettings = z.object({
  provider: z.string(),
  model: z.string().default(""),
  api_key: z.string().default(""),
  base_url: z.string().default(""),
  effort: z.string().default(""),
  max_tokens: count.default(0),
  request_timeout_seconds: count.default(0),
  idle_timeout_seconds: count.default(0),
  replay_dir: z.string().default(""),
  log_provider_errors: z.boolean().default(false),
  session_renew_bytes: count.default(0),
})
export type ModelSettings = z.infer<typeof modelSettings>

export const offeredRun = z.object({
  id: z.string().min(1),
  attempt: z.number().int().min(1),
  kind: z.string(),
  source: z.string().default(""),
  prompt: z.string().default(""),
  dashboard_id: z.string().nullish(),
})
export type OfferedRun = z.infer<typeof offeredRun>

export const runSessionOffer = z.object({
  url: z.string().min(1),
  token: z.string().min(1),
  expires_at: z.string().optional(),
})
export type RunSessionOffer = z.infer<typeof runSessionOffer>

export const claimAnswer = z.object({
  run: offeredRun.nullable(),
  session: runSessionOffer.nullish(),
  lease_seconds: count.optional(),
  retry_after_seconds: count.optional(),
  model: modelSettings.nullish(),
})
export type ClaimAnswer = z.infer<typeof claimAnswer>

export const briefAnswer = z.object({
  system: z.string(),
  message: z.string(),
  reason: z.string().default(""),
  build: buildProgress.nullish(),
})
export type BriefAnswer = z.infer<typeof briefAnswer>

export const reportAnswer = z.object({
  state: z.string(),
  stop: z.boolean().default(false),
  build: buildProgress.nullish(),
})
export type ReportAnswer = z.infer<typeof reportAnswer>

export const finishAnswer = z.object({
  state: z.string(),
  result_kind: z.string().nullish(),
  restarted: z.boolean().default(false),
})
export type FinishAnswer = z.infer<typeof finishAnswer>

export const usage = z.strictObject({
  provider: z.string(),
  model: z.string(),
  input_tokens: count,
  output_tokens: count,
  cache_read_tokens: count,
  cache_write_tokens: count,
  cost_usd: z.number().nonnegative().optional(),
  model_calls: count,
  retries: count,
  compactions: count,
  renewals: count,
  model_ms: count,
  max_save_output_tokens: count,
})
export type Usage = z.infer<typeof usage>

export const claimInput = z.strictObject({
  wait_seconds: z.number().int().min(0).max(30),
})
export type ClaimInput = z.infer<typeof claimInput>

const runRef = {
  run_id: z.string().min(1),
  attempt: z.number().int().min(1),
}

export const briefInput = z.strictObject({
  ...runRef,
  reason: z.enum(briefReasons).optional(),
})
export type BriefInput = z.infer<typeof briefInput>

export const reportInput = z.strictObject({
  ...runRef,
  text: z.string().optional(),
  usage: usage.optional(),
})
export type ReportInput = z.infer<typeof reportInput>

export const finishInput = z.strictObject({
  ...runRef,
  outcome: z.enum(["succeeded", "failed"]),
  failure_code: z.enum(loopFailureCodes).optional(),
  message: z.string().optional(),
  commit: z.boolean().optional(),
  usage,
})
export type FinishInput = z.infer<typeof finishInput>

export class ToolError extends Error {
  readonly code: string

  constructor(code: string, message: string) {
    super(message)
    this.name = "ToolError"
    this.code = code
  }
}

const toolErrorBody = z.object({
  code: z.string().min(1),
  message: z.string().default(""),
})

function parseJson(text: string): unknown {
  try {
    return JSON.parse(text)
  } catch {
    return undefined
  }
}

export function parseToolError(text: string): ToolError {
  const body = toolErrorBody.safeParse(parseJson(text))
  return body.success
    ? new ToolError(body.data.code, body.data.message)
    : new ToolError("tool_error", text.trim())
}
