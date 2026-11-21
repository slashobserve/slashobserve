export type CommentKeyInput = {
  commenting: boolean
  panelId: string | undefined
  canComment: boolean
  editing: boolean
  onCanvas: boolean
}

export type CommentKeyAction =
  | { kind: "leaveMode" }
  | { kind: "panelDraft"; panelId: string }
  | { kind: "panelInPane"; panelId: string }
  | { kind: "enterMode" }
  | { kind: "pane" }

export function commentKeyAction({
  commenting,
  panelId,
  canComment,
  editing,
  onCanvas,
}: CommentKeyInput): CommentKeyAction {
  if (commenting) return { kind: "leaveMode" }
  if (!canComment || editing) return { kind: "pane" }
  if (panelId !== undefined) {
    return onCanvas
      ? { kind: "panelDraft", panelId }
      : { kind: "panelInPane", panelId }
  }
  return onCanvas ? { kind: "enterMode" } : { kind: "pane" }
}
