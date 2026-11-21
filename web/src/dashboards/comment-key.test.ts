import { describe, expect, it } from "vitest"

import {
  commentKeyAction,
  takesCanvasComments,
  type CanvasPlaceInput,
  type CommentKeyInput,
} from "./comment-key"

const base: CommentKeyInput = {
  commenting: false,
  panelId: undefined,
  canComment: true,
  editing: false,
  onCanvas: true,
}

describe("commentKeyAction", () => {
  it("comments on the panel under the pointer", () => {
    expect(commentKeyAction({ ...base, panelId: "p1" })).toEqual({
      kind: "panel",
      panelId: "p1",
    })
  })

  it("enters Comment mode with no panel", () => {
    expect(commentKeyAction(base)).toEqual({ kind: "enterMode" })
  })

  it("leaves Comment mode when pressed again, over a panel or not", () => {
    expect(commentKeyAction({ ...base, commenting: true })).toEqual({
      kind: "leaveMode",
    })
    expect(
      commentKeyAction({ ...base, commenting: true, panelId: "p1" })
    ).toEqual({ kind: "leaveMode" })
  })

  it("opens the activity pane with no panel where the canvas takes no comments", () => {
    expect(commentKeyAction({ ...base, onCanvas: false })).toEqual({
      kind: "pane",
    })
  })

  it("only opens the activity pane while the layout has unsaved moves", () => {
    expect(commentKeyAction({ ...base, editing: true, panelId: "p1" })).toEqual(
      { kind: "pane" }
    )
  })

  it("only opens the activity pane for someone who cannot comment", () => {
    expect(
      commentKeyAction({ ...base, canComment: false, panelId: "p1" })
    ).toEqual({ kind: "pane" })
  })
})

describe("takesCanvasComments", () => {
  const onGrid: CanvasPlaceInput = {
    canComment: true,
    onTv: false,
    viewing: false,
    editingPanel: false,
    copy: false,
  }

  it("takes canvas comments on a panel in the dashboard grid", () => {
    expect(takesCanvasComments(onGrid)).toBe(true)
  })

  it.each([
    ["without Comment permission", { canComment: false }],
    ["in TV mode", { onTv: true }],
    ["in the full-size view", { viewing: true }],
    ["while the panel's editor is open", { editingPanel: true }],
    ["on a repeated panel's copy", { copy: true }],
  ])("sends comments to the activity pane %s", (_, over) => {
    expect(takesCanvasComments({ ...onGrid, ...over })).toBe(false)
  })
})
