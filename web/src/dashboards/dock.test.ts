import { describe, expect, test } from "vitest"

import { dockOf, isSidePanelCard, type DockState } from "./dock"
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
