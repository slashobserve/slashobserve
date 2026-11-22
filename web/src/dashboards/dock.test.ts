import { describe, expect, test } from "vitest"

import {
  arrangingPaneAfter,
  arrangingSetsPaneAside,
  dockOf,
  heldPaneOf,
  isSidePanelCard,
  type DockState,
} from "./dock"
import type { DashboardSearch } from "./search"

const closed: DockState = {
  open: false,
  layoutEditing: false,
  inspectable: false,
  specOpen: false,
}

function view(
  search: Partial<DashboardSearch>,
  state: Partial<DockState> = {}
) {
  return dockOf({ vars: {}, ...search }, { ...closed, ...state })
}

describe("dockOf", () => {
  test.each<
    [
      string,
      Partial<DashboardSearch>,
      Partial<DockState>,
      ReturnType<typeof dockOf>,
    ]
  >([
    ["nothing open", {}, {}, null],
    ["the side panel left open", {}, { open: true }, "activity"],
    ["the Activity tab", { tab: "activity" }, {}, "activity"],
    ["a thread", { thread: "t1" }, {}, "activity"],
    ["the Chat tab", { tab: "chat" }, {}, "chat"],
    ["a chat with no tab", { chat: "c1" }, {}, "chat"],
    [
      "a chat with no tab over an open panel",
      { chat: "c1" },
      { open: true },
      "chat",
    ],
    [
      "a chat kept behind the Activity tab",
      { chat: "c1", tab: "activity" },
      {},
      "activity",
    ],
    ["share over the Chat tab", { share: true, tab: "chat" }, {}, "share"],
    [
      "the panel editor over share",
      { panel: "p1", edit: "query", share: true },
      {},
      "edit",
    ],
    ["a panel without the editor", { panel: "p1" }, { open: true }, "activity"],
    ["the Spec over the Chat tab", { tab: "chat" }, { specOpen: true }, "spec"],
    [
      "the panel editor over the Spec",
      { panel: "p1", edit: "json" },
      { specOpen: true },
      "edit",
    ],
    [
      "inspect over the Spec",
      {},
      { inspectable: true, specOpen: true },
      "inspect",
    ],
    [
      "unsaved layout moves over the Chat tab",
      { tab: "chat" },
      { layoutEditing: true },
      null,
    ],
    [
      "unsaved layout moves over inspect",
      {},
      { layoutEditing: true, inspectable: true },
      null,
    ],
    [
      "the Spec during unsaved layout moves",
      {},
      { layoutEditing: true, specOpen: true },
      "spec",
    ],
    [
      "TV mode over the Chat tab",
      { kiosk: true, tab: "chat" },
      { open: true },
      null,
    ],
  ])("%s", (_name, search, state, expected) => {
    expect(view(search, state)).toBe(expected)
  })
})

describe("isSidePanelCard", () => {
  test.each([
    ["chat", true],
    ["activity", true],
    ["edit", true],
    ["share", true],
    ["inspect", false],
    ["spec", false],
    [null, false],
  ] as const)("%s is %s", (dock, expected) => {
    expect(isSidePanelCard(dock)).toBe(expected)
  })
})

describe("heldPaneOf", () => {
  test("keeps the Activity or chat pane held while unsaved moves hide it", () => {
    const search = { vars: {}, tab: "chat" as const, chat: "c1" }
    const editing = { ...closed, open: true, layoutEditing: true }
    expect(dockOf(search, editing)).toBeNull()
    expect(heldPaneOf(search, editing)).toBe("chat")
    expect(heldPaneOf({ vars: {} }, editing)).toBe("activity")
  })

  test("holds no share, edit or closed pane", () => {
    expect(heldPaneOf({ vars: {}, share: true }, closed)).toBeNull()
    expect(
      heldPaneOf({ vars: {}, panel: "p1", edit: "query" }, closed)
    ).toBeNull()
    expect(heldPaneOf({ vars: {} }, closed)).toBeNull()
  })
})

describe("arrangingSetsPaneAside", () => {
  test.each<[ReturnType<typeof dockOf>, boolean, boolean]>([
    ["activity", true, true],
    ["chat", true, true],
    ["activity", false, false],
    ["share", true, false],
    ["edit", true, false],
    ["inspect", true, false],
    [null, true, false],
  ])("%s beside the grid %s sets it aside: %s", (shown, beside, expected) => {
    expect(arrangingSetsPaneAside(shown, beside)).toBe(expected)
  })
})

describe("arrangingPaneAfter", () => {
  const quiet = { arranging: true, setsAside: false, reopened: false }

  test("sets the pane aside only when the session starts on a press that sets it aside", () => {
    expect(arrangingPaneAfter(null, { ...quiet, setsAside: true })).toBe(
      "aside"
    )
    expect(arrangingPaneAfter(null, quiet)).toBe("kept")
  })

  test("keeps the session's choice while the selection moves, disappears or comes back", () => {
    expect(arrangingPaneAfter("aside", quiet)).toBe("aside")
    expect(arrangingPaneAfter("kept", { ...quiet, setsAside: true })).toBe(
      "kept"
    )
  })

  test("keeps a pane the person opened again for the rest of the session", () => {
    expect(arrangingPaneAfter("aside", { ...quiet, reopened: true })).toBe(
      "kept"
    )
    expect(
      arrangingPaneAfter(null, { ...quiet, setsAside: true, reopened: true })
    ).toBe("kept")
  })

  test("ends with the session, so the next session decides again", () => {
    expect(
      arrangingPaneAfter("aside", { ...quiet, arranging: false })
    ).toBeNull()
    expect(
      arrangingPaneAfter("kept", { ...quiet, arranging: false })
    ).toBeNull()
  })
})
