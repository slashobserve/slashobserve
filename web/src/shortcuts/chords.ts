export type ShortcutAction =
  | "view"
  | "edit"
  | "inspect"
  | "close"
  | "zoomOut"
  | "shiftBack"
  | "shiftForward"
  | "refresh"
  | "tvMode"
  | "fullscreen"
  | "help"

export type Shortcut = {
  action: ShortcutAction
  keys: readonly string[]
  description: string
}

export const dashboardShortcuts: readonly Shortcut[] = [
  {
    action: "view",
    keys: ["v"],
    description: "View the panel under the pointer at full size",
  },
  {
    action: "edit",
    keys: ["e"],
    description: "Edit the panel under the pointer",
  },
  {
    action: "inspect",
    keys: ["i"],
    description: "Inspect the panel under the pointer",
  },
  {
    action: "close",
    keys: ["Escape"],
    description:
      "Close the panel view or Inspect, leave TV mode and full screen, go back from the Comment tool, or clear the selected panel",
  },
  {
    action: "zoomOut",
    keys: ["t", "z"],
    description: "Zoom the time range out",
  },
  {
    action: "shiftBack",
    keys: ["t", "ArrowLeft"],
    description: "Move the time range back",
  },
  {
    action: "shiftForward",
    keys: ["t", "ArrowRight"],
    description: "Move the time range forward",
  },
  { action: "refresh", keys: ["d", "r"], description: "Refresh every panel" },
  {
    action: "tvMode",
    keys: ["d", "k"],
    description: "Enter TV mode in full screen",
  },
  {
    action: "fullscreen",
    keys: ["f"],
    description: "Go full screen in TV mode opened from a link",
  },
  { action: "help", keys: ["?"], description: "Show keyboard shortcuts" },
]

export const chordWindowMs = 1000

export type ChordState = { pending: string | null; at: number }

export const idleChord: ChordState = { pending: null, at: 0 }

const modifierKeys = new Set(["Shift", "Control", "Alt", "Meta", "CapsLock"])

function normalized(key: string): string {
  return key.length === 1 ? key.toLowerCase() : key
}

function sameKeys(keys: readonly string[], typed: readonly string[]): boolean {
  return (
    keys.length === typed.length && keys.every((key, i) => key === typed[i])
  )
}

function actionFor(typed: readonly string[]): ShortcutAction | null {
  return (
    dashboardShortcuts.find((shortcut) => sameKeys(shortcut.keys, typed))
      ?.action ?? null
  )
}

function startsChord(key: string): boolean {
  return dashboardShortcuts.some(
    (shortcut) => shortcut.keys.length > 1 && shortcut.keys[0] === key
  )
}

export function feedKey(
  state: ChordState,
  rawKey: string,
  at: number
): { state: ChordState; action: ShortcutAction | null } {
  if (modifierKeys.has(rawKey)) return { state, action: null }
  const key = normalized(rawKey)
  if (state.pending !== null && at - state.at <= chordWindowMs) {
    const action = actionFor([state.pending, key])
    if (action !== null) return { state: idleChord, action }
  }
  const action = actionFor([key])
  if (action !== null) return { state: idleChord, action }
  if (startsChord(key)) return { state: { pending: key, at }, action: null }
  return { state: idleChord, action: null }
}

const keyLabels: Record<string, string> = {
  Escape: "Esc",
  ArrowLeft: "←",
  ArrowRight: "→",
}

export function keyLabel(key: string): string {
  return keyLabels[key] ?? key
}

export function shortcutText(action: ShortcutAction): string {
  const shortcut = dashboardShortcuts.find((entry) => entry.action === action)
  return shortcut ? shortcut.keys.map(keyLabel).join(" ") : ""
}
