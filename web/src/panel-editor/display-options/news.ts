import type { DisplayOption } from "./types"

export const news: readonly DisplayOption[] = [
  {
    kind: "text",
    key: "feedUrl",
    label: "Feed URL",
    placeholder: "https://example.com/feed.xml",
    maxLength: 2000,
  },
]
