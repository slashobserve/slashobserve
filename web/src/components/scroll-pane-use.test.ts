import { describe, expect, it } from "vitest"

import activityFeed from "@/activity/activity-feed.tsx?raw"
import conversation from "@/chat/conversation.tsx?raw"
import discussion from "@/discussion/discussion.tsx?raw"
import panelEditor from "@/panel-editor/panel-editor.tsx?raw"
import sharePane from "@/share/share-pane.tsx?raw"

const sources = {
  "chat/conversation.tsx": conversation,
  "activity/activity-feed.tsx": activityFeed,
  "discussion/discussion.tsx": discussion,
  "panel-editor/panel-editor.tsx": panelEditor,
  "share/share-pane.tsx": sharePane,
}

describe("dock panes", () => {
  it.each(Object.entries(sources))(
    "%s scrolls inside a ScrollPane",
    (_, text) => {
      expect(text).toContain("<ScrollPane")
      expect(text).not.toMatch(/<div[^>]*overflow-y-auto/)
    }
  )
})
