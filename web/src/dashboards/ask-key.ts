export type AskKeyInput = {
  asking: boolean
  panelId: string | undefined
  panelAskable: boolean
  mayAsk: boolean
  editing: boolean
  onCanvas: boolean
}

export type AskKeyAction =
  | { kind: "leaveMode" }
  | { kind: "panel"; panelId: string }
  | { kind: "enterMode" }
  | { kind: "none" }

export function askKeyAction({
  asking,
  panelId,
  panelAskable,
  mayAsk,
  editing,
  onCanvas,
}: AskKeyInput): AskKeyAction {
  if (asking) return { kind: "leaveMode" }
  if (!mayAsk || editing) return { kind: "none" }
  if (panelId !== undefined) {
    return panelAskable ? { kind: "panel", panelId } : { kind: "none" }
  }
  return onCanvas ? { kind: "enterMode" } : { kind: "none" }
}
