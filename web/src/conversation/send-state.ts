import type { ComposerModel, Failure } from "./use-composer"

export type Started = NonNullable<ComposerModel["started"]>

export type SendState =
  | { status: "idle" }
  | { status: "holding"; hold: number }
  | { status: "sending" }
  | { status: "opening"; started: Started | null }
  | { status: "failed"; failure: Failure }
  | { status: "started"; started: Started }

export type PlacedSend = { place: string; holds: number; send: SendState }

export type SendEvent =
  | { type: "placed"; place: string }
  | { type: "hold_started"; place: string }
  | { type: "hold_ended"; place: string; hold: number }
  | { type: "hold_cancelled"; place: string }
  | { type: "edited"; place: string }
  | { type: "failed"; place: string; failure: Failure | null }
  | { type: "send_started"; place: string }
  | { type: "delivered"; place: string; started: Started | null }
  | { type: "opened"; place: string }

const idle: SendState = { status: "idle" }

export function sendAt(place: string): PlacedSend {
  return { place, holds: 0, send: idle }
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
    case "opened":
      if (send.status !== "opening") return send
      return send.started === null
        ? idle
        : { status: "started", started: send.started }
  }
}

export function reduceSend(state: PlacedSend, event: SendEvent): PlacedSend {
  if (event.type === "placed") {
    return event.place === state.place
      ? state
      : { ...state, place: event.place, send: idle }
  }
  if (event.place !== state.place) return state
  const send = next(state, event)
  if (send === state.send) return state
  return {
    ...state,
    send,
    holds: send.status === "holding" ? send.hold : state.holds,
  }
}
