import { describe, expect, it } from "vitest"

import {
  cursorOverride,
  cursorSyncLabel,
  cursorSyncOptions,
  isCursorSync,
  isCursorSyncShortcut,
  nextCursorSync,
  savedCursorSync,
} from "./cursor-sync"

describe("cursorSyncOptions", () => {
  it("lists the modes in cycle order with their menu labels", () => {
    expect(cursorSyncOptions).toEqual([
      { value: "off", label: "This chart only" },
      { value: "crosshair", label: "Shared crosshair" },
      { value: "tooltip", label: "Shared tooltip" },
    ])
  })

  it("labels each mode", () => {
    expect(cursorSyncLabel("tooltip")).toBe("Shared tooltip")
  })
})

describe("isCursorSync", () => {
  it.each([
    ["off", true],
    ["crosshair", true],
    ["tooltip", true],
    ["shared", false],
    ["", false],
    [undefined, false],
    [1, false],
  ])("%j -> %s", (value, expected) => {
    expect(isCursorSync(value)).toBe(expected)
  })
})

describe("savedCursorSync", () => {
  it("uses the spec value and falls back to off", () => {
    expect(savedCursorSync({ cursorSync: "tooltip" })).toBe("tooltip")
    expect(savedCursorSync({})).toBe("off")
  })
})

describe("nextCursorSync", () => {
  it("cycles off, crosshair, tooltip and back to off", () => {
    expect(nextCursorSync("off")).toBe("crosshair")
    expect(nextCursorSync("crosshair")).toBe("tooltip")
    expect(nextCursorSync("tooltip")).toBe("off")
  })
})

describe("cursorOverride", () => {
  it("drops a choice equal to the saved mode", () => {
    expect(cursorOverride("crosshair", "crosshair")).toBeUndefined()
  })

  it("keeps a choice that differs from the saved mode", () => {
    expect(cursorOverride("off", "crosshair")).toBe("off")
    expect(cursorOverride("tooltip", "off")).toBe("tooltip")
  })
})

function element(tagName: string, isContentEditable = false) {
  return Object.assign(new EventTarget(), { tagName, isContentEditable })
}

describe("isCursorSyncShortcut", () => {
  const base = {
    key: "o",
    metaKey: false,
    ctrlKey: true,
    altKey: false,
    shiftKey: false,
    target: null,
    defaultPrevented: false,
    repeat: false,
  }

  it("matches Ctrl+O and Cmd+O", () => {
    expect(isCursorSyncShortcut(base)).toBe(true)
    expect(
      isCursorSyncShortcut({ ...base, ctrlKey: false, metaKey: true })
    ).toBe(true)
  })

  it.each([
    ["without a modifier", { ctrlKey: false }],
    ["with shift", { shiftKey: true }],
    ["with alt", { altKey: true }],
    ["for another key", { key: "k" }],
    ["when already handled", { defaultPrevented: true }],
    ["when held down", { repeat: true }],
    ["in a text field", { target: element("INPUT") }],
    ["in an editable region", { target: element("DIV", true) }],
  ])("ignores the chord %s", (_name, change) => {
    expect(isCursorSyncShortcut({ ...base, ...change })).toBe(false)
  })

  it("matches on a non-editable element", () => {
    expect(
      isCursorSyncShortcut({
        ...base,
        target: element("BUTTON"),
      })
    ).toBe(true)
  })
})
