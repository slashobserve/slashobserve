import type { AgentSettings, TurnMeta } from "../protocol"
import * as answer from "./answer"
import { type Churn, noteChurn } from "./churn"
import { readKey, writeKey } from "./key"
import {
  baseStage,
  type Loop,
  noteLedger,
  startLoop,
  stageOf,
  statusOpen,
} from "./loop"
import * as text from "./text"
import type {
  Admission,
  Call,
  CallResult,
  GuardUsage,
  Hypothesis,
  Mode,
  Settled,
  Stage,
  StepPhase,
  Stop,
  ToolPhase,
  TurnEnd,
  Unchecked,
} from "./types"

export type StreakKind = "generic" | "length" | "restricted"
export type Streak = { kind: StreakKind; count: number }

export type Latches = {
  absence: boolean
  evidence: boolean
  title: boolean
  ledger: boolean
  grounding: boolean
}

type Window = {
  calls: number
  stalled: number
  diagnose: boolean
  refused: number
  noted: number
  ledgerTurns: number
}

type TurnState = {
  position: number
  readGuard: text.GuardName | ""
  saveSeen: boolean
  answered: number
  progress: boolean
  rejected: boolean
  diagnoseNext: boolean
  noted: boolean
  ledgerSentBack: boolean
  ledgerChanged: boolean
  reads: number
  readsInFlight: number
  keys: Record<string, { position: number; refused: boolean }>
}

export type GuardState = {
  mode: Mode
  calls: { answer: number; save: number; suggest: number }
  building: boolean
  saved: boolean
  streak: Streak | null
  rejectedTotal: number
  window: Window
  lastGuard: text.GuardName | ""
  lastLimit: number
  churn: Churn
  reads: Record<string, { times: number; replayable: boolean }>
  saves: string[]
  writeKeys: string[]
  writes: number
  unchecked: Array<Unchecked & { tool: string }>
  evidence: answer.Evidence
  sentBack: Latches
  watch: { ledger: boolean; grounding: boolean }
  ledger: Hypothesis[]
  loop: Loop | null
  turn: TurnState
  counts: {
    nudges: number
    saveNowRefusals: number
    stalledTurns: number
    onePanelRefusals: number
    repairs: number
  }
}

export type GuardOptions = {
  mode: Mode
  agent: AgentSettings
  task?: string
  investigation: boolean
  building: boolean
}

export type PhaseOf = (name: string) => ToolPhase

const saveTools = new Set([
  "create_dashboard",
  "update_dashboard",
  "update_panels",
  "import_grafana_dashboard",
  "save_finding",
])
const queryTools = new Set([
  "query_range",
  "query_instant",
  "measure_window",
  "measure_aggregate",
])

export function toolPhase(name: string, readOnly: boolean): ToolPhase {
  if (saveTools.has(name)) {
    return "save"
  }
  if (name === "note_hypotheses") {
    return "ledger"
  }
  if (name === "lint_query") {
    return "lint"
  }
  if (queryTools.has(name)) {
    return "query"
  }
  return readOnly ? "discover" : "write"
}

const saveFinding = "save_finding"

function freshWindow(): Window {
  return {
    calls: 0,
    stalled: 0,
    diagnose: false,
    refused: 0,
    noted: 0,
    ledgerTurns: 0,
  }
}

function freshTurn(readGuard: text.GuardName | ""): TurnState {
  return {
    position: 0,
    readGuard,
    saveSeen: false,
    answered: 0,
    progress: false,
    rejected: false,
    diagnoseNext: false,
    noted: false,
    ledgerSentBack: false,
    ledgerChanged: false,
    reads: 0,
    readsInFlight: 0,
    keys: {},
  }
}

export function newGuardState(options: GuardOptions): GuardState {
  const { mode, agent } = options
  const investigation =
    agent.investigation_calls ||
    (options.task === undefined ? 0 : (agent.task_calls[options.task] ?? 0)) ||
    agent.default_calls ||
    text.defaultInvestigationCalls
  const answerCalls =
    mode === "investigate"
      ? investigation
      : agent.chat_calls || text.defaultChatCalls
  const looping = agent.loop && mode === "investigate" && options.investigation
  return {
    mode,
    calls: {
      answer: answerCalls,
      save: agent.save_calls || text.defaultSaveCalls,
      suggest: agent.suggest_calls || text.defaultSuggestCalls,
    },
    building: options.building,
    saved: false,
    streak: null,
    rejectedTotal: 0,
    window: freshWindow(),
    lastGuard: "",
    lastLimit: 0,
    churn: { changes: {}, inRow: 0 },
    reads: {},
    saves: [],
    writeKeys: [],
    writes: 0,
    unchecked: [],
    evidence: answer.noEvidence(),
    sentBack: {
      absence: false,
      evidence: false,
      title: false,
      ledger: false,
      grounding: false,
    },
    watch: {
      ledger: looping,
      grounding: mode !== "suggest" && agent.grounding === "enforce",
    },
    ledger: [],
    loop: looping
      ? startLoop(
          answerCalls,
          agent.investigation_calls_max || text.defaultInvestigationCallsMax
        )
      : null,
    turn: freshTurn(""),
    counts: {
      nudges: 0,
      saveNowRefusals: 0,
      stalledTurns: 0,
      onePanelRefusals: 0,
      repairs: 0,
    },
  }
}

function oneLine(value: unknown): string {
  return typeof value === "string"
    ? value.split(/\s+/).filter(Boolean).join(" ")
    : ""
}

function trimmed(value: unknown): string {
  return typeof value === "string" ? value.trim() : ""
}

export function ledgerOf(input: string): Hypothesis[] {
  let parsed: unknown
  try {
    parsed = JSON.parse(input)
  } catch {
    return []
  }
  const items = (parsed as { hypotheses?: unknown } | null)?.hypotheses
  if (!Array.isArray(items)) {
    return []
  }
  return items.map((item: Record<string, unknown>) => {
    const status = typeof item.status === "string" ? item.status : ""
    const h: Hypothesis = {
      id: trimmed(item.id),
      text: oneLine(item.text),
      status,
    }
    const closing = trimmed(item.closing)
    const knockOnOf = trimmed(item.knock_on_of)
    const evidence = Array.isArray(item.evidence)
      ? item.evidence.map(oneLine)
      : []
    const next = status === statusOpen ? oneLine(item.next_check) : ""
    if (closing !== "") h.closing = closing
    if (knockOnOf !== "") h.knock_on_of = knockOnOf
    if (evidence.length > 0) h.evidence = evidence
    if (next !== "") h.next = next
    return h
  })
}

type StreakStop = {
  limit: number
  failure: Stop["failure"]
  step: (mode: Mode) => string
  message: string
}

const streakStops: Record<StreakKind, StreakStop> = {
  restricted: {
    limit: text.maxRejectionsInRow,
    failure: "restricted_answer",
    step: (mode) =>
      `Stopped: ${text.maxRejectionsInRow} ${text.rejectedThing(mode)} in a row were refused because the run read data only Admins can read`,
    message: "",
  },
  length: {
    limit: text.maxLengthRejections,
    failure: "validation_exhausted",
    step: () =>
      `Stopped: ${text.maxLengthRejections} findings in a row were over the word limit`,
    message: `the finding was still over the word limit after ${text.maxLengthRejections} saves`,
  },
  generic: {
    limit: text.maxRejectionsInRow,
    failure: "validation_exhausted",
    step: (mode) =>
      `Stopped: ${text.maxRejectionsInRow} ${text.rejectedThing(mode)} in a row were rejected`,
    message: "",
  },
}

type SaveOutcome =
  | {
      kind: "accepted"
      as: "built" | "saved" | "finding" | "suggested" | "proposed"
    }
  | { kind: "rejected"; streak: StreakKind }
  | { kind: "neutral" }

function errorCode(content: string): string {
  try {
    const parsed: unknown = JSON.parse(content)
    const code = (parsed as { code?: unknown } | null)?.code
    return typeof code === "string" ? code : ""
  } catch {
    return ""
  }
}

export function saveOutcomeOf(result: CallResult): SaveOutcome {
  if (result.isError) {
    if (result.settled !== null) {
      return { kind: "neutral" }
    }
    switch (errorCode(result.content)) {
      case text.codeRestrictedAnswer:
        return { kind: "rejected", streak: "restricted" }
      case text.codeFindingTooLong:
        return { kind: "rejected", streak: "length" }
    }
    return { kind: "rejected", streak: "generic" }
  }
  switch (result.settled) {
    case null:
      return { kind: "accepted", as: "built" }
    case "finding":
    case "suggested":
    case "proposed":
    case "saved":
      return { kind: "accepted", as: result.settled }
  }
  return { kind: "neutral" }
}

export class Guard {
  readonly state: GuardState
  private readonly phaseOf: PhaseOf

  constructor(state: GuardState, phaseOf: PhaseOf) {
    this.state = state
    this.phaseOf = phaseOf
  }

  static create(options: GuardOptions, phaseOf: PhaseOf): Guard {
    return new Guard(newGuardState(options), phaseOf)
  }

  snapshot(): GuardState {
    return structuredClone(this.state)
  }

  limit(): number {
    const s = this.state
    switch (s.mode) {
      case "suggest":
        return s.calls.suggest
      case "investigate":
        return s.loop?.limit ?? s.calls.answer
    }
    return this.answering() ? s.calls.answer : s.calls.save
  }

  phase(name: string): StepPhase {
    return this.repairing() && this.phaseOf(name) === "save"
      ? "repair"
      : this.phaseOf(name)
  }

  stage(): Stage | "" {
    const loop = this.state.loop
    return loop === null ? "" : stageOf(loop, this.state.window.calls)
  }

  usage(): GuardUsage {
    const s = this.state
    const usage: GuardUsage = {
      nudges: s.counts.nudges,
      save_now_refusals: s.counts.saveNowRefusals,
      stalled_turns: s.counts.stalledTurns,
      one_panel_refusals: s.counts.onePanelRefusals,
      repair_rounds: s.counts.repairs,
    }
    if (s.loop !== null) {
      usage.call_limit = s.loop.limit
      if (s.loop.savedStage !== "") {
        usage.stage_at_save = s.loop.savedStage
      }
    }
    if (s.ledger.length > 0) {
      usage.hypotheses = structuredClone(s.ledger)
    }
    return usage
  }

  beginSession(building: boolean): void {
    this.state.reads = {}
    this.state.window = freshWindow()
    this.state.building = building
  }

  restart(): void {
    const s = this.state
    s.streak = null
    s.rejectedTotal = 0
    s.churn = { changes: {}, inRow: 0 }
    s.saves = []
    s.evidence = answer.noEvidence()
    s.sentBack.absence = false
    s.sentBack.evidence = false
    s.sentBack.title = false
    this.beginSession(false)
  }

  seedLedger(ledger: Hypothesis[]): void {
    this.state.ledger = structuredClone(ledger)
    const loop = this.state.loop
    if (loop !== null) {
      loop.base = baseStage(ledger)
      loop.candidates = ledger.map((h) => h.id)
    }
  }

  beginTurn(turn: Pick<TurnMeta, "saves">): void {
    this.state.window.calls++
    this.state.turn = freshTurn(this.readRefusal(turn.saves > 0))
  }

  admit(call: Call): Admission {
    const s = this.state
    const adm: Admission = {
      verdict: "dispatch",
      phase: this.phase(call.name),
      key: this.keyOf(call),
      position: s.turn.position++,
    }
    switch (this.phaseOf(call.name)) {
      case "save":
        return this.admitSave(call, adm)
      case "write":
        return this.admitWrite(call, adm)
      case "ledger":
        s.turn.noted = true
        return adm
    }
    const read = this.admitRead(call, adm)
    if (read.verdict !== "refuse") {
      s.turn.readsInFlight++
      if (s.loop !== null) {
        s.turn.reads++
      }
    }
    return read
  }

  endRead(call: Call): void {
    const phase = this.phaseOf(call.name)
    if (phase === "save" || phase === "write" || phase === "ledger") {
      return
    }
    if (this.state.turn.readsInFlight > 0) {
      this.state.turn.readsInFlight--
    }
  }

  forget(call: Call): void {
    const key = this.keyOf(call)
    const s = this.state
    switch (this.phaseOf(call.name)) {
      case "write":
        if (s.writeKeys.includes(key)) {
          s.writeKeys = s.writeKeys.filter((k) => k !== key)
          s.writes--
        }
        return
      case "save":
        s.saves = s.saves.filter((k) => k !== key)
    }
  }

  settle(call: Call, adm: Admission, result: CallResult): Settled {
    if (adm.verdict !== "refuse") {
      this.endRead(call)
    }
    if (adm.verdict !== "dispatch") {
      return {}
    }
    const s = this.state
    switch (this.phaseOf(call.name)) {
      case "write":
        if (result.isError) {
          this.forget(call)
        }
        return {}
      case "ledger":
        if (result.isError) {
          s.turn.ledgerSentBack = true
        } else {
          this.noteLedger(ledgerOf(call.input))
        }
        return {}
      case "save":
        return this.settleSave(result)
    }
    this.settleRead(call, adm, result)
    return {}
  }

  endTurn(): TurnEnd {
    const s = this.state
    s.window.diagnose = s.turn.rejected || s.turn.diagnoseNext
    if (this.stalled()) {
      return {
        nudge: "",
        stageLine: "",
        stageStep: "",
        notes: [],
        stop: {
          failure: "no_progress",
          step: this.stalledReason(),
          message: "",
        },
      }
    }
    let nudge = ""
    if (!s.turn.progress) {
      nudge = this.nudge()
      if (nudge !== "") {
        s.counts.nudges++
      }
    }
    nudge = this.loopNudge(nudge)
    const stop = this.rejectionStop()
    if (stop !== undefined) {
      return {
        nudge,
        stageLine: "",
        stageStep: "",
        notes: nudge === "" ? [] : [nudge],
        stop,
      }
    }
    const stageLine = this.stageLine()
    const stageStep = stageLine === "" ? "" : this.stageChange()
    return {
      nudge,
      stageLine,
      stageStep,
      notes: [nudge, stageLine].filter((note) => note !== ""),
    }
  }

  private keyOf(call: Call): string {
    return this.phaseOf(call.name) === "write"
      ? writeKey(call.name, call.input)
      : readKey(call.name, call.input)
  }

  private answering(): boolean {
    const s = this.state
    return s.mode === "create" && !s.building && !s.saved
  }

  private repairing(): boolean {
    const streak = this.state.streak
    return streak !== null && streak.kind !== "restricted"
  }

  private rule(name: text.GuardName, calls = this.limit()): text.Rule {
    return text.ruleOf(
      name,
      calls === 0 ? this.limit() : calls,
      this.answering()
    )
  }

  private readRefusal(hasSave: boolean): text.GuardName | "" {
    const s = this.state
    if (hasSave || s.window.diagnose || s.window.calls < this.limit()) {
      return ""
    }
    switch (s.mode) {
      case "suggest":
        return "propose_now"
      case "investigate":
        return "finding_now"
    }
    return "save_now"
  }

  private nudge(): string {
    const s = this.state
    if (s.window.calls !== this.limit() - 1) {
      return ""
    }
    switch (s.mode) {
      case "suggest":
        return text.messageProposeNext
      case "investigate":
        return text.messageFindingNext
    }
    return this.answering() ? text.messageAnswerNext : text.messageSaveNext
  }

  private refuse(call: Call, adm: Admission, name: text.GuardName): Admission {
    const s = this.state
    s.lastGuard = name
    s.lastLimit = this.limit()
    const rule = this.rule(name)
    return {
      ...adm,
      verdict: "refuse",
      content: text.refusalContent(text.codeOf(name), rule.message),
      step: `Refused ${call.name}: ${rule.refused}`,
    }
  }

  private reject(
    call: Call,
    adm: Admission,
    reason: string,
    people: string
  ): Admission {
    return {
      ...adm,
      verdict: "refuse",
      content: text.refusalContent(text.codeInvalidRequest, reason),
      step: text.sentBackToolStep(call.name, people),
    }
  }

  private admitRead(call: Call, adm: Admission): Admission {
    const s = this.state
    if (s.turn.readGuard !== "") {
      s.counts.saveNowRefusals++
      s.window.refused++
      return this.refuse(call, adm, s.turn.readGuard)
    }
    const first = s.turn.keys[adm.key]
    if (first !== undefined) {
      if (!first.refused) {
        s.turn.answered++
      }
      return {
        ...adm,
        verdict: "reuse",
        reuseOf: first.position,
        step: text.stepReusedRead,
      }
    }
    const entry = s.reads[adm.key] ?? { times: 0, replayable: false }
    entry.times++
    s.reads[adm.key] = entry
    if (entry.times >= text.maxIdenticalReads) {
      s.turn.keys[adm.key] = { position: adm.position, refused: true }
      return this.refuse(call, adm, "repeated_read")
    }
    s.turn.keys[adm.key] = { position: adm.position, refused: false }
    s.turn.answered++
    if (entry.replayable) {
      return {
        ...adm,
        verdict: "replay",
        step: text.stepReusedRead,
        note: text.messageReplayed,
      }
    }
    return adm
  }

  private admitWrite(call: Call, adm: Admission): Admission {
    const s = this.state
    if (s.turn.readsInFlight > 0) {
      return this.refuse(call, adm, "read_in_flight")
    }
    if (s.writes >= text.maxWritesPerAttempt) {
      return this.refuse(call, adm, "too_many_writes")
    }
    if (s.writeKeys.includes(adm.key)) {
      return this.refuse(call, adm, "repeated_write")
    }
    s.writeKeys.push(adm.key)
    s.writes++
    s.turn.answered++
    return adm
  }

  private admitSave(call: Call, adm: Admission): Admission {
    const s = this.state
    if (s.turn.saveSeen) {
      return this.reject(
        call,
        adm,
        "only one save per turn",
        "it was a second save in the same turn"
      )
    }
    if (call.name === saveFinding && s.building) {
      s.turn.saveSeen = true
      return this.reject(
        call,
        adm,
        "this run already saved panels; finish the dashboard instead of saving a finding",
        "this run is building a dashboard, so it cannot save an answer"
      )
    }
    if (call.name === saveFinding) {
      const faults = this.unbackedFinding(call)
      if (faults.length > 0) {
        s.turn.saveSeen = true
        s.turn.answered++
        return {
          ...adm,
          verdict: "refuse",
          content: text.refusalContent(
            text.codeInvalidRequest,
            faults.map((f) => f.model).join("; and ")
          ),
          step:
            text.sentBackStep + text.peopleText(faults.map((f) => f.people)),
        }
      }
    }
    s.turn.saveSeen = true
    if (s.saves.includes(adm.key)) {
      return this.refuse(call, adm, "repeated_save")
    }
    s.saves.push(adm.key)
    s.turn.answered++
    if (call.name === saveFinding) {
      return {
        ...adm,
        readsRefused: s.window.refused,
        unchecked: this.uncheckedReads(),
      }
    }
    return adm
  }

  private unbackedFinding(call: Call): answer.Fault[] {
    const s = this.state
    const finding = answer.findingOf(call.input)
    const faults: answer.Fault[] = []
    const take = (latch: keyof Latches, fault: answer.Fault | undefined) => {
      if (fault !== undefined && !s.sentBack[latch]) {
        s.sentBack[latch] = true
        faults.push(fault)
      }
    }
    take("absence", answer.absenceFault(s.evidence, finding))
    take("evidence", answer.evidenceFault(s.evidence, finding))
    take("title", answer.titleFault(finding))
    if (s.watch.ledger && !s.sentBack.ledger) {
      take("ledger", this.ledgerFault(call, finding))
    }
    if (s.watch.grounding && call.ungrounded) {
      take("grounding", {
        model: call.ungrounded,
        people: text.peopleUngrounded,
        needsRead: true,
      })
    }
    if (faults.some((f) => f.needsRead)) {
      s.turn.diagnoseNext = true
    }
    return faults
  }

  private ledgerFault(
    call: Call,
    finding: answer.Finding
  ): answer.Fault | undefined {
    const s = this.state
    if (call.unclosed && s.window.calls < this.limit()) {
      s.counts.nudges++
      return {
        model: call.unclosed,
        people: text.peopleUnclosed,
        needsRead: true,
      }
    }
    const resolved = s.ledger.some((h) => h.status !== statusOpen)
    if (resolved || !finding.conclusion) {
      return undefined
    }
    s.counts.nudges++
    return {
      model: text.faultLedgerBehind,
      people: text.peopleLedgerBehind,
      needsRead: false,
    }
  }

  private uncheckedReads(): Unchecked[] {
    const out: Unchecked[] = []
    for (const u of this.state.unchecked) {
      if (
        !out.some((o) => o.datasource === u.datasource && o.code === u.code)
      ) {
        out.push({ datasource: u.datasource, code: u.code })
      }
    }
    return out
  }

  private noteLedger(next: Hypothesis[]): void {
    const s = this.state
    if (s.loop !== null && noteLedger(s.loop, s.ledger, next)) {
      s.turn.ledgerChanged = true
    }
    s.ledger = next
  }

  private settleRead(call: Call, adm: Admission, result: CallResult): void {
    const s = this.state
    const phase = this.phaseOf(call.name)
    if (s.loop !== null && !result.isError) {
      s.loop.readSinceGrowth = true
    }
    if (phase !== "lint") {
      this.noteChecked(call, result)
    }
    if (call.name === "list_metrics") {
      if (answer.inputField(call.input, "scope").trim() !== "") {
        s.evidence.scopedMetrics = true
      }
    }
    if (call.name === "list_annotations" && !result.isError) {
      s.evidence.annotations = true
    }
    if (!result.isError && answer.readsData(call.name, phase)) {
      s.evidence.queried = true
    }
    const entry = s.reads[adm.key]
    if (entry !== undefined && !result.isError && phase !== "query") {
      entry.replayable = true
    }
  }

  private noteChecked(call: Call, result: CallResult): void {
    const datasource = answer.inputField(call.input, "datasource")
    if (datasource === "") {
      return
    }
    const s = this.state
    const same = (u: { tool: string; datasource: string }) =>
      u.tool === call.name && u.datasource === datasource
    const code = result.call?.unchecked ?? null
    if (code !== null && code !== "") {
      const known = s.unchecked.find(same)
      if (known !== undefined) {
        known.code = code
      } else if (s.unchecked.length < text.maxUnchecked) {
        s.unchecked.push({ tool: call.name, datasource, code })
      }
    } else if (!result.isError) {
      s.unchecked = s.unchecked.filter((u) => !same(u))
    }
  }

  private settleSave(result: CallResult): Settled {
    const s = this.state
    const stage = this.stage()
    if (result.call?.refusal === "one_panel") {
      s.counts.onePanelRefusals++
    }
    const outcome = saveOutcomeOf(result)
    if (outcome.kind === "neutral") {
      s.turn.progress = false
      s.turn.rejected = false
      return {}
    }
    if (outcome.kind === "rejected") {
      const kind = outcome.streak
      s.streak = {
        kind,
        count: s.streak?.kind === kind ? s.streak.count + 1 : 1,
      }
      if (kind !== "restricted") {
        s.rejectedTotal++
      }
      s.turn.progress = false
      s.turn.rejected = true
      return {}
    }
    if (this.repairing()) {
      s.counts.repairs++
    }
    s.streak = null
    s.rejectedTotal = 0
    s.turn.rejected = false
    s.turn.progress = true
    let settled: Settled = {}
    switch (outcome.as) {
      case "saved":
        s.saved = true
        s.window = freshWindow()
        break
      case "finding":
        if (s.loop !== null && stage !== "") {
          s.loop.savedStage = stage
        }
        s.window = freshWindow()
        break
      case "proposed":
        s.window = freshWindow()
        break
      case "built": {
        s.building = true
        s.window = freshWindow()
        s.reads = {}
        const churn = noteChurn(s.churn, result.call?.edits)
        if (churn.stop) {
          s.turn.progress = false
          return {
            stop: {
              failure: "no_progress",
              step: `Stopped: ${text.maxChurnedSavesInRow + 1} saves in a row changed nothing new`,
              message: "",
            },
          }
        }
        if (churn.note !== "") {
          settled = { note: churn.note }
        }
      }
    }
    return settled
  }

  private stalled(): boolean {
    const s = this.state
    const turn = s.turn
    if (turn.answered > 0) {
      s.window.stalled = 0
      s.window.noted = 0
      s.window.ledgerTurns = 0
    } else if (
      (turn.ledgerSentBack || turn.ledgerChanged) &&
      s.window.ledgerTurns < text.maxLedgerTurns
    ) {
      s.window.ledgerTurns++
    } else {
      s.counts.stalledTurns++
      if (this.onlyNoted()) {
        s.window.noted++
      }
      s.window.stalled++
    }
    return s.window.stalled >= text.maxStalledTurns
  }

  private onlyNoted(): boolean {
    const turn = this.state.turn
    return (
      turn.noted &&
      !turn.rejected &&
      !Object.values(turn.keys).some((key) => key.refused)
    )
  }

  private stalledReason(): string {
    const s = this.state
    if (s.window.noted > 0 && s.window.noted === s.window.stalled) {
      return `Stopped: ${text.maxStalledTurns} turns in a row only updated the hypotheses`
    }
    if (s.lastGuard !== "") {
      return this.rule(s.lastGuard, s.lastLimit).stopped
    }
    return `Stopped: ${text.maxStalledTurns} turns in a row had every call refused`
  }

  private loopNudge(limitNudge: string): string {
    const s = this.state
    const loop = s.loop
    if (loop === null || s.turn.progress) {
      return limitNudge
    }
    if (s.turn.ledgerChanged) {
      loop.quietTurns = 0
      loop.reminded = false
    } else if (s.turn.reads > 0) {
      loop.quietTurns++
    } else {
      loop.quietTurns = 0
    }
    if (loop.base !== "conclude") {
      loop.concludeSent = false
    }
    if (limitNudge !== "") {
      return limitNudge
    }
    if (loop.base === "conclude" && !loop.concludeSent) {
      loop.concludeSent = true
      return text.messageConclude
    }
    if (
      loop.base !== "" &&
      loop.quietTurns >= text.quietTurnsToNudge &&
      !loop.reminded
    ) {
      loop.reminded = true
      return text.messageReminder
    }
    return ""
  }

  private rejectionStop(): Stop | undefined {
    const s = this.state
    if (!s.turn.rejected) {
      return undefined
    }
    if (s.streak !== null) {
      const row = streakStops[s.streak.kind]
      if (s.streak.count >= row.limit) {
        return {
          failure: row.failure,
          step: row.step(s.mode),
          message: row.message,
        }
      }
    }
    if (s.rejectedTotal >= text.maxRejectedSaves) {
      return {
        failure: "validation_exhausted",
        step: `Stopped: ${text.maxRejectedSaves} ${text.rejectedThing(s.mode)} were rejected without an accepted one`,
        message: `the finding was rejected in ${text.maxRejectedSaves} saves without an accepted one`,
      }
    }
    return undefined
  }

  private stageLine(): string {
    const s = this.state
    if (s.loop === null || s.turn.progress) {
      return ""
    }
    return `Stage: ${this.stage()}. Model call ${s.window.calls} of ${s.loop.limit}.`
  }

  private stageChange(): string {
    const s = this.state
    const stage = this.stage()
    if (
      s.loop === null ||
      s.turn.progress ||
      stage === "" ||
      s.loop.shown === stage
    ) {
      return ""
    }
    s.loop.shown = stage
    return `Stage: ${stage}`
  }
}
