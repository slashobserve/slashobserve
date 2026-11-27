import { z } from "zod"

export const turnMetaKey = "slashobserve.com/turn"
export const runMetaKey = "slashobserve.com/run"
export const callMetaKey = "slashobserve.com/call"

export type CallHints = { sweep?: boolean; step?: string }

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

export const guardFailureCodes = [
  "validation_exhausted",
  "no_progress",
  "restricted_answer",
] as const
export type GuardFailureCode = (typeof guardFailureCodes)[number]

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

export const settledKinds = [
  "saved",
  "proposed",
  "finding",
  "suggested",
  "restarted",
  "ended",
] as const
export type SettledKind = (typeof settledKinds)[number]

export const callMeta = z.object({
  edits: z
    .object({
      panels: z.array(z.string()),
      added: z.array(z.string()),
      deleted: z.array(z.string()),
      settings: z.boolean(),
    })
    .nullable(),
  refusal: z.string().nullable(),
  unchecked: z.string().nullable(),
  discovered: z
    .object({
      datasource: z.string(),
      of: z.string(),
      names: z.array(z.string()),
    })
    .nullable(),
  ranges: z.array(z.object({ from: z.string(), to: z.string() })),
})
export type CallMeta = z.infer<typeof callMeta>

export const runMeta = z.object({
  settled: z.enum(settledKinds).nullable(),
  build: buildProgress.nullable(),
  call: callMeta.optional(),
})
export type RunMeta = z.infer<typeof runMeta>

export const noRunMeta: RunMeta = { settled: null, build: null }

export const defaultMaxTokens = 16000
export const defaultAnthropicEffort = "medium"
export const defaultRequestTimeoutSeconds = 600
export const defaultIdleTimeoutSeconds = 180

export const modelSettings = z
  .object({
    provider: z.string(),
    model: z.string().default(""),
    api_key: z.string().default(""),
    base_url: z.string().default(""),
    effort: z.string().default(""),
    max_tokens: count.default(0),
    request_timeout_seconds: count.default(defaultRequestTimeoutSeconds),
    idle_timeout_seconds: count.default(defaultIdleTimeoutSeconds),
    replay_dir: z.string().default(""),
    log_provider_errors: z.boolean().default(false),
    session_renew_bytes: count.default(0),
  })
  .transform((settings) => ({
    ...settings,
    effort:
      settings.effort === "" && settings.provider === "anthropic"
        ? defaultAnthropicEffort
        : settings.effort,
    max_tokens:
      settings.max_tokens === 0 ? defaultMaxTokens : settings.max_tokens,
  }))
export type ModelSettings = z.infer<typeof modelSettings>

export const offeredRun = z.object({
  id: z.string().min(1),
  attempt: z.number().int().min(1),
  claim_epoch: z.number().int().min(1).optional(),
  kind: z.string(),
  intent: z.string().default(""),
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

export const agentSettings = z.object({
  investigation_calls: count.default(0),
  investigation_calls_max: count.default(0),
  default_calls: count.default(0),
  task_calls: z.record(z.string(), count).default({}),
  chat_calls: count.default(0),
  save_calls: count.default(0),
  suggest_calls: count.default(0),
  loop: z.boolean().default(false),
  grounding: z.string().default(""),
  brief_sweep: z.boolean().default(false),
  session_renew_bytes: count.default(0),
})
export type AgentSettings = z.infer<typeof agentSettings>

export const claimAnswer = z.object({
  run: offeredRun.nullable(),
  session: runSessionOffer.nullish(),
  lease_seconds: count.optional(),
  retry_after_seconds: count.optional(),
  model: modelSettings.nullish(),
  agent: agentSettings.nullish().catch(null),
})
export type ClaimAnswer = z.infer<typeof claimAnswer>

const unverified = z.object({
  panels: z.array(z.unknown()).nullish(),
  variables: z.array(z.unknown()).nullish(),
})

export const runBrief = z.object({
  request: z.object({
    mode: z.string(),
    source: z.string(),
    prompt: z.string(),
    panel_id: z.string().nullish(),
    from: z.string().nullish(),
    to: z.string().nullish(),
    time_zone: z.string().nullish(),
    zone: z
      .object({ label: z.string(), offset_seconds: z.number().int() })
      .nullish(),
    as_of: z.string().nullish(),
    continue_note: z.string().nullish(),
    thread_context: z.string().nullish(),
    continues: z.boolean().default(false),
    attempt: z.number().int().min(1),
    investigation: z.boolean().default(false),
    observations: z.array(z.string()).default([]),
    pointer: z.object({
      anchor: z.unknown().nullish(),
      block_id: z.string().nullish(),
      observation_id: z.string().nullish(),
      thread_ask: z.boolean().default(false),
    }),
  }),
  current: z
    .object({
      dashboard_id: z.string(),
      number: count,
      spec: z.unknown(),
      unverified,
      conversion: z.array(z.unknown()).nullish(),
    })
    .nullish(),
  build: z
    .object({
      base_version: count,
      spec: z.unknown(),
      unverified,
      progress: buildProgress,
    })
    .nullish(),
  datasources: z.array(
    z.object({
      uid: z.string(),
      type: z.string(),
      name: z.string().default(""),
      spans: z.string().default(""),
      healthy: z.boolean(),
      everyone: z.boolean().default(false),
    })
  ),
  withheld: z.boolean().default(false),
  guide: z.object({ text: z.string(), schema: z.unknown() }),
  draft: z.unknown().nullish(),
})
export type RunBrief = z.infer<typeof runBrief>

export const agentState = z.object({
  session: z.unknown().optional(),
  carry: z.unknown().optional(),
})
export type AgentState = z.infer<typeof agentState>

export const briefAnswer = z.object({
  reason: z.string().default(""),
  build: buildProgress.nullish(),
  brief: runBrief.nullish().catch(null),
  agent_state: agentState.nullish().catch(null),
})
export type BriefAnswer = z.infer<typeof briefAnswer>

export const reportAnswer = z.object({
  state: z.string(),
  stop: z.boolean().default(false),
  lease_until: z.string().nullish(),
  server_time: z.string().nullish(),
  build: buildProgress.nullish(),
})
export type ReportAnswer = z.infer<typeof reportAnswer>

export const releaseAnswer = z.object({
  state: z.string(),
})
export type ReleaseAnswer = z.infer<typeof releaseAnswer>

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
  failed_calls: count,
  retries: count,
  compactions: count,
  renewals: count,
  model_ms: count,
  max_save_output_tokens: count,
  nudges: count.optional(),
  save_now_refusals: count.optional(),
  stalled_turns: count.optional(),
  one_panel_refusals: count.optional(),
  repair_rounds: count.optional(),
  hypotheses: z.array(z.record(z.string(), z.unknown())).max(6).optional(),
  call_limit: count.optional(),
  stage_at_save: z
    .enum(["orient", "hypothesize", "test", "conclude"])
    .optional(),
})
export type Usage = z.infer<typeof usage>

export const turnMeta = z.strictObject({
  id: z.string().min(1),
  calls: z.number().int().min(1),
  saves: count,
  usage: usage.optional(),
})
export type TurnMeta = z.infer<typeof turnMeta>

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

export const maxAgentStateBytes = 256 << 10
export const maxReportedSteps = 20

export const stepPhases = [
  "discover",
  "lint",
  "query",
  "save",
  "write",
  "ledger",
  "repair",
  "stage",
] as const

export const reportedStep = z.strictObject({
  phase: z.enum(stepPhases),
  tool: z.string().max(64).optional(),
  status: z.enum(["ok", "error"]),
  message: z.string().max(500),
})
export type ReportedStep = z.infer<typeof reportedStep>

export const reportInput = z.strictObject({
  ...runRef,
  text: z.string().optional(),
  usage: usage.optional(),
  agent_state: agentState.optional(),
  steps: z.array(reportedStep).max(maxReportedSteps).optional(),
  task: z
    .enum(["measure", "logs", "traces", "incident", "inventory", "explain"])
    .optional(),
})
export type ReportInput = z.infer<typeof reportInput>

export const releaseInput = z.strictObject({
  ...runRef,
  claim_epoch: z.number().int().min(1).optional(),
})
export type ReleaseInput = z.infer<typeof releaseInput>

export const finishInput = z.strictObject({
  ...runRef,
  outcome: z.enum(["succeeded", "failed"]),
  failure_code: z.enum([...loopFailureCodes, ...guardFailureCodes]).optional(),
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
