import type { ComposerModel, Failure } from "./use-composer"

export type Started = NonNullable<ComposerModel["started"]>

export type SendState =
  | { status: "idle" }
  | { status: "holding"; hold: number }
  | { status: "sending" }
  | { status: "opening"; started: Started | null }
  | { status: "failed"; failure: Failure }
  | { status: "started"; started: Started }

export type PlacedSend = {
  place: string
  holds: number
  send: SendState
  clientId: string
  sentBodies: readonly string[]
}

export type SendEvent =
  | { type: "placed"; place: string; clientId: string }
  | { type: "hold_started"; place: string }
  | { type: "hold_ended"; place: string; hold: number }
  | { type: "hold_cancelled"; place: string }
  | { type: "edited"; place: string }
  | { type: "failed"; place: string; failure: Failure | null }
  | { type: "send_started"; place: string; body: string }
  | {
      type: "delivered"
      place: string
      started: Started | null
      clientId: string
    }
  | { type: "superseded"; place: string; failure: Failure; clientId: string }
  | { type: "opened"; place: string }

const idle: SendState = { status: "idle" }

export function sendAt(place: string, clientId: string): PlacedSend {
  return { place, holds: 0, send: idle, clientId, sentBodies: [] }
}

export function sentOtherBody(state: PlacedSend, body: string): boolean {
  return state.sentBodies.some((sent) => sent !== body)
}

export function sendIn(state: PlacedSend, place: string): SendState {
  return state.place === place ? state.send : idle
}

export function delivering(send: SendState): boolean {
  return send.status === "sending" || send.status === "opening"
}

function next(state: PlacedSend, event: SendEvent): SendState {
  const { send } = state
  switch (event.type) {
    case "placed":
      return idle
    case "hold_started":
      return send.status === "holding" || delivering(send)
        ? send
        : { status: "holding", hold: state.holds + 1 }
    case "hold_ended":
      return send.status === "holding" && send.hold === event.hold ? idle : send
    case "hold_cancelled":
      return send.status === "holding" ? idle : send
    case "edited":
      return send.status === "failed" ? idle : send
    case "failed":
      if (send.status === "opening") return send
      return event.failure === null
        ? idle
        : { status: "failed", failure: event.failure }
    case "send_started":
      return delivering(send) ? send : { status: "sending" }
    case "delivered":
      return send.status === "sending"
        ? { status: "opening", started: event.started }
        : send
    case "superseded":
      return send.status === "sending"
        ? { status: "failed", failure: event.failure }
        : send
    case "opened":
      if (send.status !== "opening") return send
      return send.started === null
        ? idle
        : { status: "started", started: send.started }
  }
}

function attempts(
  state: PlacedSend,
  event: SendEvent
): Pick<PlacedSend, "clientId" | "sentBodies"> {
  switch (event.type) {
    case "send_started":
      return {
        clientId: state.clientId,
        sentBodies: [...state.sentBodies, event.body],
      }
    case "delivered":
    case "superseded":
      return { clientId: event.clientId, sentBodies: [] }
    default:
      return state
  }
}

export function reduceSend(state: PlacedSend, event: SendEvent): PlacedSend {
  if (event.type === "placed") {
    return event.place === state.place
      ? state
      : {
          ...state,
          place: event.place,
          send: idle,
          clientId: event.clientId,
          sentBodies: [],
        }
  }
  if (event.place !== state.place) return state
  const send = next(state, event)
  if (send === state.send) return state
  const { clientId, sentBodies } = attempts(state, event)
  return {
    ...state,
    send,
    holds: send.status === "holding" ? send.hold : state.holds,
    clientId,
    sentBodies,
  }
}
