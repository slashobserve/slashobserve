import { describe, expect, it } from "vitest"

import activityFeed from "@/activity/activity-feed.tsx?raw"
import conversation from "@/chat/conversation.tsx?raw"
import discussion from "@/discussion/discussion.tsx?raw"

const sources = {
  "chat/conversation.tsx": conversation,
  "activity/activity-feed.tsx": activityFeed,
  "discussion/discussion.tsx": discussion,
}

describe("dock transcripts", () => {
  it.each(Object.entries(sources))(
    "%s scrolls inside a ScrollPane",
    (_, text) => {
      expect(text).toContain("<ScrollPane")
      expect(text).not.toMatch(/<div[^>]*overflow-y-auto/)
    }
  )
})
