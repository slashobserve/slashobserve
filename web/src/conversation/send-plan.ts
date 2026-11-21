import type { ObserveInput } from "@/observe/queries"
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
  target?: SpotTarget
  replyToId?: string
  range?: { from: string; to: string }
  revises?: boolean
}

export type Opens = "chat" | "same"

export type Send =
  | {
      via: "thread.open"
      target: string
      body: string
      to: Delivery["to"]
      clientId: string
      anchor?: Anchor
      panelId?: string
      blockId?: string
    }
  | {
      via: "thread.reply"
      threadId: string
      body: string
      to: Delivery["to"]
      clientId: string
      blockId?: string
      replyToId?: string
    }
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

function subjectFields(subject: Subject | null): Partial<ObserveInput> {
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
): Partial<ObserveInput> {
  if (subject === null || target === undefined) return {}
  const fields: Partial<ObserveInput> = {}
  if (subject.kind === "dashboard" && target.panelId !== undefined) {
    fields.panelId = target.panelId
  }
  if (target.anchor !== undefined) {
    fields.anchor = target.anchor
    if (subject.kind === "observation" && target.blockId !== undefined) {
      fields.blockId = target.blockId
    }
  }
  return fields
}

function chatTurn(place: Place, subject: Subject | null, draft: Draft): Send {
  const chatId = place.kind === "chat" ? place.chatId : null
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
        ...(subject?.kind === "dashboard" ? { dashboardId: subject.id } : {}),
        ...(subject?.kind === "dashboard" && draft.target?.panelId
          ? { panelId: draft.target.panelId }
          : {}),
        ...(draft.range ?? {}),
      }
      return { via: "chat.turn", input, opens }
    }
    case null: {
      if (draft.revises === true && subject?.kind === "dashboard") {
        return {
          via: "chat.turn",
          input: { ...base, ...chat, kind: "revise", dashboardId: subject.id },
          opens,
        }
      }
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

function threadSend(
  place: Place,
  subject: Subject | null,
  delivery: Delivery,
  draft: Draft
): Planned {
  if (place.kind === "thread" && place.threadId !== null) {
    return {
      via: "thread.reply",
      threadId: place.threadId,
      body: draft.body,
      to: delivery.to,
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
    to: delivery.to,
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

const reviseLead = "Revise the dashboard: "

export function planRevise(
  place: Place,
  subject: Subject | null,
  change: string,
  clientId: string
): Planned {
  if (subject?.kind !== "dashboard") {
    return { error: "Only a dashboard can be revised." }
  }
  const body = `${reviseLead}${change}`
  if (place.kind === "thread" && place.threadId !== null) {
    return {
      via: "thread.reply",
      threadId: place.threadId,
      body,
      to: "agent",
      clientId,
    }
  }
  const chat: Place =
    place.kind === "chat" ? place : { kind: "chat", chatId: null }
  return chatTurn(chat, subject, { body, clientId, revises: true })
}
