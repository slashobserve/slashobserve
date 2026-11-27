import { addDiscovery, type Discovery } from "./brief/digest"
import { byteLength } from "./brief/gofmt"
import { type Message, render } from "./brief/render"
import type { Reason } from "./brief/seed"
import {
  checkSave,
  type GroundingRecord,
  noteResult,
  parseMode,
  type SaveCheck,
  savedFinding,
  startGrounding,
} from "./grounding/grounding"
import {
  type Admission,
  type Call,
  Guard,
  type GuardState,
  type Mode,
  toolPhase,
} from "./guard"
import { log } from "./log"
import type { RunTools, ToolReply } from "./mcp"
import { isSave, readsData, sweepTools } from "./phase"
import {
  type AgentSettings,
  type BriefAnswer,
  type BriefReason,
  briefReasons,
  maxAgentStateBytes,
  type ReportedStep,
  type RunBrief,
  type Usage,
} from "./protocol"
import type { AgentNotes } from "./reporter"
import { classify, full, parseTask, type Task, taskName } from "./route/route"
import type { ToolCall, ToolSpec } from "./session/types"
import {
  checkCandidates,
  leadsOf,
  noteTransient,
  seedLedger,
} from "./triage/conclude"
import { ToolReader } from "./triage/reader"
import { defaultLimits, sourcesOf, sweep, sweptStep } from "./triage/sweep"
import type { Lead } from "./triage/types"
import {
  type Admitted,
  type Args,
  type Settled,
  type Stop,
  type Watch,
  withNotes,
} from "./turn"

export function stepRouted(task: Task): string {
  return `Routed the question to the ${taskName(task)} task type`
}

const saveFinding = "save_finding"
const noteHypotheses = "note_hypotheses"
const maxStoredDigestBytes = 8 << 10
const sweepReadTimeoutMs = defaultLimits.time

type Triage = { block: string; leads: Lead[]; calls: number; duration: number }

type Stored = {
  version: 1
  task: Task
  guard: GuardState
  grounding: GroundingRecord | null
  triage: Triage | null
  digest: Discovery[]
  dataReads: number
  transientAt: Record<string, number>
}

type Carried = { triage: Triage | null }

export type StartInput = {
  mode: Mode
  prompt: string
  agent: AgentSettings
  answer: BriefAnswer
  tools: RunTools
  signal: AbortSignal
}

function storedOf(raw: unknown): Stored | undefined {
  const s = raw as Partial<Stored> | null | undefined
  if (
    typeof s !== "object" ||
    s === null ||
    s.version !== 1 ||
    typeof s.guard !== "object" ||
    s.guard === null ||
    parseTask(String(s.task ?? "")) === undefined
  ) {
    return undefined
  }
  return s as Stored
}

function carriedOf(raw: unknown): Carried {
  const c = raw as Partial<Carried> | null | undefined
  return { triage: c?.triage ?? null }
}

function reasonOf(raw: string): Reason {
  return (briefReasons as readonly string[]).includes(raw)
    ? (raw as BriefReason)
    : ""
}

function timeOf(text: string | null | undefined): number | undefined {
  const at = Date.parse(text ?? "")
  return Number.isNaN(at) ? undefined : at
}

function keptDigest(entries: Discovery[]): Discovery[] {
  let bytes = 0
  let kept = 0
  for (let i = entries.length - 1; i >= 0; i--) {
    const e = entries[i] as Discovery
    bytes += byteLength(e.datasource) + byteLength(e.of)
    for (const name of e.names) {
      bytes += byteLength(name)
    }
    if (bytes > maxStoredDigestBytes) {
      break
    }
    kept++
  }
  return entries.slice(entries.length - kept)
}

function savedPanels(answer: BriefAnswer): boolean {
  return (answer.brief?.build?.progress.saves ?? answer.build?.saves ?? 0) > 0
}

function argsOf(value: unknown): Args {
  return typeof value === "object" && value !== null && !Array.isArray(value)
    ? (value as Args)
    : {}
}

type CallState = {
  call: Call
  admission: Admission
  check?: SaveCheck
  candidates?: ReturnType<typeof checkCandidates>
}

export class RunState implements Watch {
  private readonly mode: Mode
  private readonly prompt: string
  private readonly agent: AgentSettings
  private readonly loop: boolean
  private readonly guard: Guard
  private task: Task
  private grounding: GroundingRecord | undefined
  private triage: Triage | null
  private digest: Discovery[]
  private dataReads: number
  private transientAt: Record<string, number>
  private specs: readonly ToolSpec[] = []
  private steps: ReportedStep[] = []
  private reportTask: Task = full
  private sentState = ""
  private readonly calls = new Map<string, CallState>()

  private constructor(
    input: StartInput,
    stored: Stored | undefined,
    brief: RunBrief | null
  ) {
    this.mode = input.mode
    this.prompt = input.prompt
    this.agent = input.agent
    this.loop =
      input.agent.loop &&
      input.mode === "investigate" &&
      (brief?.request.investigation ?? false)
    const phaseOf = (name: string) =>
      toolPhase(
        name,
        this.specs.find((spec) => spec.name === name)?.readOnly !== false
      )
    const mode = parseMode(input.agent.grounding) ?? "off"
    if (stored !== undefined) {
      this.task = stored.task
      this.guard = new Guard(stored.guard, phaseOf)
      this.grounding =
        input.mode === "suggest"
          ? undefined
          : (stored.grounding ?? startGrounding(mode))
      this.triage = stored.triage
      this.digest = stored.digest
      this.dataReads = stored.dataReads
      this.transientAt = stored.transientAt
      return
    }
    this.task =
      input.mode === "investigate" && brief !== null
        ? classify({
            investigation: brief.request.investigation,
            dashboard: brief.current != null,
            panel: brief.request.panel_id != null,
            observation: brief.request.observations.length > 0,
            types: brief.datasources.map((d) => d.type),
            text: brief.request.prompt,
          })
        : full
    this.guard = Guard.create(
      {
        mode: input.mode,
        agent: input.agent,
        ...(input.mode === "investigate" ? { task: this.task } : {}),
        investigation: brief?.request.investigation ?? false,
        building: savedPanels(input.answer),
      },
      phaseOf
    )
    this.grounding = input.mode === "suggest" ? undefined : startGrounding(mode)
    this.triage = carriedOf(input.answer.agent_state?.carry).triage
    this.digest = []
    this.dataReads = 0
    this.transientAt = {}
    if (input.mode === "investigate") {
      this.steps.push({
        phase: "discover",
        status: "ok",
        message: stepRouted(this.task),
      })
      this.reportTask = this.task
    }
  }

  static async start(input: StartInput): Promise<RunState> {
    const brief = input.answer.brief ?? null
    const stored = storedOf(input.answer.agent_state?.session)
    const state = new RunState(input, stored, brief)
    if (stored === undefined) {
      await state.preflight(input, brief)
      state.seed()
    }
    return state
  }

  private async preflight(
    input: StartInput,
    brief: RunBrief | null
  ): Promise<void> {
    if (
      brief === null ||
      input.mode !== "investigate" ||
      !brief.request.investigation ||
      !input.agent.brief_sweep ||
      this.triage !== null
    ) {
      return
    }
    const r = brief.request
    const from = timeOf(r.from)
    const to = timeOf(r.to)
    const sources = sourcesOf(brief.datasources).sort((a, b) =>
      a.uid < b.uid ? -1 : a.uid > b.uid ? 1 : 0
    )
    const reader = new ToolReader(async (name, args, abort) => {
      const reply = await input.tools.callTool(
        name,
        args,
        undefined,
        sweepReadTimeoutMs,
        AbortSignal.any([abort, input.signal]),
        { sweep: true }
      )
      return { content: reply.content, isError: reply.isError }
    })
    const out = await sweep(reader, {
      sources,
      end: to ?? timeOf(r.as_of) ?? Date.now(),
      question:
        from !== undefined && to !== undefined ? { from, to } : undefined,
    })
    input.signal.throwIfAborted()
    this.triage = {
      block: out.block,
      leads: out.leads,
      calls: out.calls,
      duration: out.duration,
    }
    this.steps.push({ phase: "query", status: "ok", message: sweptStep(out) })
  }

  private seed(): void {
    const seeded = seedLedger(this.leads(), this.guard.state.ledger)
    if (seeded !== undefined) {
      this.guard.seedLedger(seeded)
    }
  }

  private leads(): Lead[] {
    return leadsOf(this.loop, this.task, this.triage?.leads)
  }

  offer(specs: readonly ToolSpec[]): ToolSpec[] {
    const offered = specs.filter((spec) => !sweepTools.has(spec.name))
    this.specs = offered
    return offered
  }

  beginSession(answer: BriefAnswer): void {
    this.guard.beginSession(savedPanels(answer))
  }

  restart(): void {
    this.guard.restart()
  }

  render(answer: BriefAnswer, now: Date): Pick<Message, "system" | "user"> {
    if (!answer.brief) {
      return { system: answer.system, user: answer.message }
    }
    return render(
      answer.brief,
      reasonOf(answer.reason),
      {
        task: this.task,
        loop: this.loop,
        facts: this.grounding !== undefined,
        ledger: this.guard.state.ledger,
        triage: this.triage?.block ?? "",
        digest: this.digest,
      },
      now
    )
  }

  usage(base: Usage): Usage {
    const g = this.guard.usage()
    const out: Usage = {
      ...base,
      nudges: g.nudges,
      save_now_refusals: g.save_now_refusals,
      stalled_turns: g.stalled_turns,
      one_panel_refusals: g.one_panel_refusals,
      repair_rounds: g.repair_rounds,
    }
    if (g.call_limit !== undefined) {
      out.call_limit = g.call_limit
    }
    if (g.stage_at_save !== undefined) {
      out.stage_at_save = g.stage_at_save
    }
    if (g.hypotheses !== undefined && g.hypotheses.length <= 6) {
      out.hypotheses = g.hypotheses as Array<Record<string, unknown>>
    }
    return out
  }

  routes(): boolean {
    return this.reportTask !== full
  }

  closing(): AgentNotes {
    const notes: AgentNotes = {}
    if (this.steps.length > 0) {
      notes.steps = this.steps
      this.steps = []
    }
    return notes
  }

  notes(): AgentNotes {
    const notes: AgentNotes = {}
    if (this.steps.length > 0) {
      notes.steps = this.steps
      this.steps = []
    }
    if (this.reportTask !== full) {
      notes.task = this.reportTask
      this.reportTask = full
    }
    const state = this.agentState()
    if (state !== this.sentState) {
      this.sentState = state
      notes.agent_state = JSON.parse(state) as AgentNotes["agent_state"]
    }
    return notes
  }

  noteStop(stop: Stop): void {
    this.steps.push({ phase: "save", status: "error", message: stop.step })
  }

  private agentState(): string {
    const stored: Stored = {
      version: 1,
      task: this.task,
      guard: this.guard.snapshot(),
      grounding: this.grounding ?? null,
      triage: this.triage,
      digest: this.digest,
      dataReads: this.dataReads,
      transientAt: this.transientAt,
    }
    const carry: Carried = { triage: this.triage }
    let text = JSON.stringify({ session: stored, carry })
    if (byteLength(text) > maxAgentStateBytes) {
      log("warn", "agent state too large, grounding record left out", {
        bytes: byteLength(text),
      })
      text = JSON.stringify({ session: { ...stored, grounding: null }, carry })
    }
    return text
  }

  beginTurn(saves: number): void {
    this.guard.beginTurn({ saves })
  }

  admit(call: ToolCall, args: Args): Admitted {
    const guarded: Call = { name: call.name, input: call.input }
    const state: CallState = {
      call: guarded,
      admission: undefined as unknown as Admission,
    }
    if (call.name === saveFinding) {
      state.check = checkSave(this.grounding, args, this.prompt, this.task)
      if (state.check.sendBack !== "") {
        guarded.ungrounded = state.check.sendBack
      }
      state.candidates = checkCandidates(
        this.leads(),
        this.guard.state.ledger,
        this.transientAt,
        args
      )
      if (state.candidates.unclosed !== "") {
        guarded.unclosed = state.candidates.unclosed
      }
    }
    const adm = this.guard.admit(guarded)
    state.admission = adm
    this.calls.set(call.id, state)
    if (adm.verdict === "refuse") {
      this.steps.push({
        phase: adm.phase,
        tool: call.name,
        status: "error",
        message: adm.step ?? "",
      })
      return { refused: true, content: adm.content ?? "" }
    }
    const hints = adm.step === undefined ? undefined : { step: adm.step }
    if (call.name !== saveFinding || state.check === undefined) {
      return { refused: false, args, hints }
    }
    const sent = argsOf(
      savedFinding(
        this.grounding,
        args,
        state.check,
        state.candidates?.unverified ?? []
      )
    )
    if (adm.readsRefused !== undefined) {
      sent["reads_refused"] = adm.readsRefused
    }
    if (adm.unchecked !== undefined) {
      sent["unchecked"] = adm.unchecked
    }
    return { refused: false, args: sent, hints }
  }

  settle(
    call: ToolCall,
    args: Args,
    _admitted: Admitted,
    reply: ToolReply
  ): Settled {
    const state = this.calls.get(call.id)
    if (state === undefined) {
      return { content: reply.content }
    }
    this.calls.delete(call.id)
    let content = reply.content
    if (!reply.isError) {
      content = noteResult(this.grounding, call.name, args, {
        content,
        isError: false,
        meta: reply.meta.call,
      })
      if (readsData(call.name)) {
        this.dataReads++
      }
    }
    const before = this.guard.state.ledger
    const settled = this.guard.settle(state.call, state.admission, {
      isError: reply.isError,
      content: reply.content,
      settled: reply.meta.settled,
      ...(reply.meta.call === undefined ? {} : { call: reply.meta.call }),
    })
    if (call.name === noteHypotheses && !reply.isError) {
      this.transientAt = noteTransient(
        this.leads(),
        before,
        this.guard.state.ledger,
        this.transientAt,
        this.dataReads
      )
    }
    const found = reply.meta.call?.discovered
    if (found && !reply.isError && !isSave(call.name)) {
      this.digest = keptDigest(addDiscovery(this.digest, found))
    }
    const notes = [state.admission.note, settled.note].filter(
      (note): note is string => note !== undefined && note !== ""
    )
    if (settled.stop !== undefined) {
      this.noteStop(settled.stop)
    }
    return { content: withNotes(content, notes), stop: settled.stop }
  }

  failed(call: ToolCall): void {
    const state = this.calls.get(call.id)
    if (state === undefined) {
      return
    }
    this.calls.delete(call.id)
    this.guard.endRead(state.call)
    this.guard.forget(state.call)
  }

  endTurn(): { notes: string[]; stop?: Stop | undefined } {
    const end = this.guard.endTurn()
    if (end.stop !== undefined) {
      this.noteStop(end.stop)
      return { notes: end.notes, stop: end.stop }
    }
    if (end.stageStep !== "") {
      this.steps.push({ phase: "stage", status: "ok", message: end.stageStep })
    }
    return { notes: end.notes }
  }
}
