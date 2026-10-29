import { describe, expect, it } from "vitest"

import {
  isEditableTarget,
  isModChord,
  isModEnter,
  modChordLabel,
} from "./shortcuts"

function keyEvent(overrides: Partial<Parameters<typeof isModChord>[0]>) {
  return {
    key: "k",
    metaKey: false,
    ctrlKey: false,
    altKey: false,
    shiftKey: false,
    ...overrides,
  }
}

describe("isEditableTarget", () => {
  it("is true for inputs, textareas, selects and contenteditable", () => {
    expect(
      isEditableTarget({ tagName: "INPUT" } as unknown as EventTarget)
    ).toBe(true)
    expect(
      isEditableTarget({ tagName: "TEXTAREA" } as unknown as EventTarget)
    ).toBe(true)
    expect(
      isEditableTarget({ tagName: "SELECT" } as unknown as EventTarget)
    ).toBe(true)
    expect(
      isEditableTarget({
        tagName: "DIV",
        isContentEditable: true,
      } as unknown as EventTarget)
    ).toBe(true)
  })

  it("is false for other elements, the document and null", () => {
    expect(
      isEditableTarget({ tagName: "BODY" } as unknown as EventTarget)
    ).toBe(false)
    expect(isEditableTarget(new EventTarget())).toBe(false)
    expect(isEditableTarget(null)).toBe(false)
  })
})

describe("isModChord", () => {
  it("accepts Meta+k and Ctrl+K", () => {
    expect(isModChord(keyEvent({ metaKey: true }), "k")).toBe(true)
    expect(isModChord(keyEvent({ ctrlKey: true, key: "K" }), "k")).toBe(true)
  })

  it("matches the key argument case-insensitively", () => {
    expect(isModChord(keyEvent({ metaKey: true }), "K")).toBe(true)
    expect(isModChord(keyEvent({ metaKey: true, key: "o" }), "O")).toBe(true)
  })

  it("rejects Ctrl+Shift+K, Alt chords, a plain k and another key", () => {
    expect(
      isModChord(keyEvent({ ctrlKey: true, shiftKey: true, key: "K" }), "k")
    ).toBe(false)
    expect(isModChord(keyEvent({ ctrlKey: true, altKey: true }), "k")).toBe(
      false
    )
    expect(isModChord(keyEvent({}), "k")).toBe(false)
    expect(isModChord(keyEvent({ metaKey: true, key: "j" }), "k")).toBe(false)
  })
})

describe("isModEnter", () => {
  it("accepts Enter with Meta or Ctrl", () => {
    expect(isModEnter(keyEvent({ key: "Enter", metaKey: true }))).toBe(true)
    expect(isModEnter(keyEvent({ key: "Enter", ctrlKey: true }))).toBe(true)
  })

  it("rejects a plain Enter and a modified other key", () => {
    expect(isModEnter(keyEvent({ key: "Enter" }))).toBe(false)
    expect(isModEnter(keyEvent({ key: "k", metaKey: true }))).toBe(false)
  })
})

describe("modChordLabel", () => {
  const mac =
    "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15"
  const windows = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/130.0"

  it("uses the command symbol on Apple devices", () => {
    expect(modChordLabel(mac, "K")).toBe("⌘K")
    expect(modChordLabel(mac, "o")).toBe("⌘O")
  })

  it("spells out Ctrl elsewhere", () => {
    expect(modChordLabel(windows, "K")).toBe("Ctrl K")
    expect(modChordLabel(windows, "o")).toBe("Ctrl O")
  })
})
