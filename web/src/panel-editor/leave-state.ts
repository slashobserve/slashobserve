export type LeaveState<Kind extends string> =
  | { status: "clean" }
  | { status: "dirty" }
  | { status: "confirming"; kind: Kind }
  | { status: "blocked" }

export type LeaveEvent<Kind extends string> =
  | { type: "edited"; dirty: boolean }
  | { type: "asked"; kind: Kind }
  | { type: "navigation_held" }
  | { type: "kept" }
  | { type: "left" }

const clean = { status: "clean" } as const
const dirty = { status: "dirty" } as const

export const noUnsavedEdits: LeaveState<never> = clean

export function reduceLeave<Kind extends string>(
  state: LeaveState<Kind>,
  event: LeaveEvent<Kind>
): LeaveState<Kind> {
  switch (event.type) {
    case "edited":
      if (state.status === "clean") return event.dirty ? dirty : state
      if (state.status === "dirty") return event.dirty ? state : clean
      return state
    case "asked":
      return state.status === "dirty"
        ? { status: "confirming", kind: event.kind }
        : state
    case "navigation_held":
      return state.status === "clean" ? state : { status: "blocked" }
    case "kept":
      return state.status === "confirming" || state.status === "blocked"
        ? dirty
        : state
    case "left":
      return clean
  }
}

export function hasUnsavedEdits(state: LeaveState<string>): boolean {
  return state.status !== "clean"
}

export function asksToDiscard(state: LeaveState<string>): boolean {
  return state.status === "confirming" || state.status === "blocked"
}
