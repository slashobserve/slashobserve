import type { ObserveInput, ObserveTurn } from "@/observe/queries"
import type { SlashCommand } from "@/chat/slash-commands"

import type { Anchor } from "./anchor"
import { storeOf, type Delivery, type Place } from "./delivery"

const subjectKinds = [
  "dashboard",
  "observation",
  "annotation",
  "proposal",
] as const

export type Subject = { kind: (typeof subjectKinds)[number]; id: string }

export type SpotTarget = { anchor?: Anchor; panelId?: string; blockId?: string }

export type PageCommand = SlashCommand

export type Draft = {
  body: string
  clientId: string
  command?: PageCommand | null
  target?: SpotTarget | undefined
  replyToId?: string | undefined
  range?: { from: string; to: string } | undefined
  toChosen?: boolean
}

export type Opens = "chat" | "same"

export type Send =
  | {
      via: "thread.open"
      target: string
      body: string
      to?: Delivery["to"]
      clientId: string
      anchor?: Anchor
      panelId?: string
      blockId?: string
    }
  | {
      via: "thread.reply"
      threadId: string
      body: string
      to?: Delivery["to"]
      clientId: string
      blockId?: string
      replyToId?: string
    }
  | { via: "thread.apply"; threadId: string; findingId: string }
  | { via: "chat.turn"; input: ObserveInput; opens: Opens }

export type Planned = Send | { error: string }

export const noteToSelfText =
  "A message only you can see has to go to the agent."
export const noSubjectText =
  "Open a dashboard or an observation to write to the team."

export function threadTargetOf(subject: Subject): string {
  return `${subject.kind}:${subject.id}`
}

export function subjectOf(target: string): Subject | null {
  const at = target.indexOf(":")
  const kind = subjectKinds.find((known) => known === target.slice(0, at))
  const id = target.slice(at + 1)
  return kind === undefined || id === "" ? null : { kind, id }
}

function subjectFields(subject: Subject | null): Partial<ObserveTurn> {
  switch (subject?.kind) {
    case "dashboard":
      return { dashboardId: subject.id }
    case "observation":
      return { observationId: subject.id }
    default:
      return {}
  }
}

function spotFields(
  subject: Subject | null,
  target: SpotTarget | undefined
): Partial<ObserveTurn> {
  if (subject === null || target === undefined) return {}
  const fields: Partial<ObserveTurn> = {}
  if (subject.kind === "dashboard" && target.panelId !== undefined) {
    fields.panelId = target.panelId
  }
  if (target.anchor !== undefined) fields.anchor = target.anchor
  if (subject.kind === "observation" && target.blockId !== undefined) {
    fields.blockId = target.blockId
  }
  return fields
}

function chatOf(place: Place): string | null {
  if (place.kind === "chat") return place.chatId
  if (place.kind === "canvas" && place.tool === "ask") return place.chatId
  return null
}

function chatTurn(place: Place, subject: Subject | null, draft: Draft): Send {
  const chatId = chatOf(place)
  const chat = chatId === null ? {} : { chatId }
  const opens: Opens = chatId === null && subject === null ? "chat" : "same"
  const base = { prompt: draft.body, clientId: draft.clientId }
  switch (draft.command ?? null) {
    case "import":
      return {
        via: "chat.turn",
        input: { ...base, kind: "ask" },
        opens: "chat",
      }
    case "investigate": {
      const input: ObserveInput = {
        ...base,
        ...chat,
        kind: "investigate",
        ...subjectFields(subject),
        ...(subject?.kind === "dashboard" && draft.target?.panelId
          ? { panelId: draft.target.panelId }
          : {}),
        ...(draft.range ?? {}),
      }
      return { via: "chat.turn", input, opens }
    }
    case null: {
      const input: ObserveInput = {
        ...base,
        ...chat,
        kind: "ask",
        ...subjectFields(subject),
        ...spotFields(subject, draft.target),
      }
      return { via: "chat.turn", input, opens }
    }
  }
}

function namedRecipient(
  place: Place,
  delivery: Delivery,
  draft: Draft
): { to?: Delivery["to"] } {
  const serverDecides = place.kind === "thread" && draft.toChosen !== true
  return serverDecides ? {} : { to: delivery.to }
}

function threadSend(
  place: Place,
  subject: Subject | null,
  delivery: Delivery,
  draft: Draft
): Planned {
  const recipient = namedRecipient(place, delivery, draft)
  if (place.kind === "thread" && place.threadId !== null) {
    return {
      via: "thread.reply",
      threadId: place.threadId,
      body: draft.body,
      ...recipient,
      clientId: draft.clientId,
      ...(draft.target?.blockId === undefined
        ? {}
        : { blockId: draft.target.blockId }),
      ...(draft.replyToId === undefined ? {} : { replyToId: draft.replyToId }),
    }
  }
  if (subject === null) return { error: noSubjectText }
  const target = draft.target ?? {}
  return {
    via: "thread.open",
    target: threadTargetOf(subject),
    body: draft.body,
    ...recipient,
    clientId: draft.clientId,
    ...(target.anchor === undefined ? {} : { anchor: target.anchor }),
    ...(subject.kind === "dashboard" && target.panelId !== undefined
      ? { panelId: target.panelId }
      : {}),
    ...(subject.kind === "observation" && target.blockId !== undefined
      ? { blockId: target.blockId }
      : {}),
  }
}

export function planSend(
  place: Place,
  subject: Subject | null,
  delivery: Delivery,
  draft: Draft
): Planned {
  switch (storeOf(delivery)) {
    case null:
      return { error: noteToSelfText }
    case "thread":
      return threadSend(place, subject, delivery, draft)
    case "chat":
      return chatTurn(place, subject, draft)
  }
}

export function planRevise(
  place: Place,
  subject: Subject | null,
  findingId: string,
  clientId: string
): Planned {
  if (subject?.kind !== "dashboard") {
    return { error: "Only a dashboard can be revised." }
  }
  if (place.kind === "thread" && place.threadId !== null) {
    return {
      via: "thread.apply",
      threadId: place.threadId,
      findingId,
    }
  }
  const chatId = chatOf(place)
  return {
    via: "chat.turn",
    input: {
      kind: "revise",
      appliesFindingId: findingId,
      dashboardId: subject.id,
      clientId,
      ...(chatId === null ? {} : { chatId }),
    },
    opens: "same",
  }
}
