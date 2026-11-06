import { describe, expect, it } from "vitest"

import {
  chordWindowMs,
  feedKey,
  idleChord,
  shortcutText,
  type ChordState,
  type ShortcutAction,
} from "./chords"

function typeKeys(keys: readonly (string | [string, number])[]) {
  let state: ChordState = idleChord
  const actions: (ShortcutAction | null)[] = []
  let at = 1000
  for (const entry of keys) {
    const [key, gap] = typeof entry === "string" ? [entry, 100] : entry
    at += gap
    const result = feedKey(state, key, at)
    state = result.state
    actions.push(result.action)
  }
  return actions
}

describe("feedKey", () => {
  it.each([
    [["v"], "view"],
    [["e"], "edit"],
    [["i"], "inspect"],
    [["Escape"], "close"],
    [["?"], "help"],
    [["t", "z"], "zoomOut"],
    [["t", "ArrowLeft"], "shiftBack"],
    [["t", "ArrowRight"], "shiftForward"],
    [["d", "r"], "refresh"],
    [["d", "k"], "tvMode"],
    [["f"], "fullscreen"],
    [["F"], "fullscreen"],
  ] as const)("maps %j to %s", (keys, action) => {
    expect(typeKeys(keys).at(-1)).toBe(action)
  })

  it("waits after the first key of a chord", () => {
    expect(typeKeys(["t"])).toEqual([null])
    expect(typeKeys(["d"])).toEqual([null])
  })

  it("treats an uppercase letter like the lowercase one", () => {
    expect(typeKeys(["V"])).toEqual(["view"])
    expect(typeKeys(["D", "R"])).toEqual([null, "refresh"])
  })

  it("ignores the Shift key pressed on the way to ?", () => {
    expect(typeKeys(["Shift", "?"])).toEqual([null, "help"])
    expect(typeKeys(["t", "Shift", "z"])).toEqual([null, null, "zoomOut"])
  })

  it("drops a chord whose second key comes after the window", () => {
    expect(typeKeys(["t", ["z", chordWindowMs + 1]])).toEqual([null, null])
    expect(typeKeys(["d", ["r", chordWindowMs]])).toEqual([null, "refresh"])
  })

  it("reads a single-key shortcut that breaks a chord on its own", () => {
    expect(typeKeys(["t", "v"])).toEqual([null, "view"])
    expect(typeKeys(["d", "Escape"])).toEqual([null, "close"])
  })

  it("starts a new chord when a chord key follows a chord key", () => {
    expect(typeKeys(["t", "d", "r"])).toEqual([null, null, "refresh"])
  })

  it.each([["z"], ["r"], ["k"], ["/"], ["Enter"], ["ArrowLeft"]])(
    "ignores %s on its own",
    (key) => {
      expect(typeKeys([key])).toEqual([null])
    }
  )

  it("forgets the pending key after an unrelated key", () => {
    expect(typeKeys(["t", "x", "z"])).toEqual([null, null, null])
  })
})

describe("shortcutText", () => {
  it.each([
    ["view", "v"],
    ["close", "Esc"],
    ["shiftBack", "t ←"],
    ["tvMode", "d k"],
  ] as const)("writes %s as %s", (action, text) => {
    expect(shortcutText(action)).toBe(text)
  })
})
