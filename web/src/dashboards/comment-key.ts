export type CommentKeyInput = {
  commenting: boolean
  panelId: string | undefined
  canComment: boolean
  editing: boolean
  onCanvas: boolean
}

export type CommentKeyAction =
  | { kind: "leaveMode" }
  | { kind: "panel"; panelId: string }
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
  if (panelId !== undefined) return { kind: "panel", panelId }
  return onCanvas ? { kind: "enterMode" } : { kind: "pane" }
}

export type CanvasPlaceInput = {
  canComment: boolean
  onTv: boolean
  viewing: boolean
  editingPanel: boolean
  copy: boolean
}

export function takesCanvasComments({
  canComment,
  onTv,
  viewing,
  editingPanel,
  copy,
}: CanvasPlaceInput): boolean {
  return canComment && !onTv && !viewing && !editingPanel && !copy
}
