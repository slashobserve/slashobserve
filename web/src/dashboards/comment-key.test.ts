import { describe, expect, it } from "vitest"

import { commentKeyAction, type CommentKeyInput } from "./comment-key"

const base: CommentKeyInput = {
  commenting: false,
  panelId: undefined,
  canComment: true,
  editing: false,
  onCanvas: true,
}

describe("commentKeyAction", () => {
  it("opens the whole-panel composer on the panel under the pointer", () => {
    expect(commentKeyAction({ ...base, panelId: "p1" })).toEqual({
      kind: "panelDraft",
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

  it("comments in the activity pane where the canvas takes no comments", () => {
    expect(
      commentKeyAction({ ...base, onCanvas: false, panelId: "p1" })
    ).toEqual({ kind: "panelInPane", panelId: "p1" })
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
