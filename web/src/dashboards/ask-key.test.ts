import { describe, expect, it } from "vitest"

import { askKeyAction, type AskKeyInput } from "./ask-key"

const idle: AskKeyInput = {
  asking: false,
  panelId: undefined,
  panelAskable: false,
  mayAsk: true,
  editing: false,
  onCanvas: true,
}

describe("askKeyAction", () => {
  it("turns Ask mode on with no panel under the pointer", () => {
    expect(askKeyAction(idle)).toEqual({ kind: "enterMode" })
  })

  it("asks about the whole panel under the pointer", () => {
    expect(
      askKeyAction({ ...idle, panelId: "p1", panelAskable: true })
    ).toEqual({ kind: "panel", panelId: "p1" })
  })

  it("leaves Ask mode when it is on, wherever the pointer is", () => {
    expect(
      askKeyAction({ ...idle, asking: true, panelId: "p1", panelAskable: true })
    ).toEqual({ kind: "leaveMode" })
  })

  it("does nothing on a panel the agent cannot read, off the canvas, while editing or with no model", () => {
    expect(askKeyAction({ ...idle, panelId: "p1" })).toEqual({ kind: "none" })
    expect(askKeyAction({ ...idle, onCanvas: false })).toEqual({
      kind: "none",
    })
    expect(askKeyAction({ ...idle, editing: true })).toEqual({ kind: "none" })
    expect(askKeyAction({ ...idle, mayAsk: false })).toEqual({ kind: "none" })
  })
})
