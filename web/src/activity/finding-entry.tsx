import { FindingCard } from "@/findings/finding-card"
import { observeAgentMark } from "@/lib/author"

import { EntryFrame } from "./entry-frame"
import type { TimelineEntry } from "./timeline-model"

type Props = {
  entry: Extract<TimelineEntry, { kind: "finding" }>
}

export function FindingEntry({ entry }: Props) {
  return (
    <EntryFrame
      avatar={observeAgentMark}
      who="Agent"
      verb="found"
      at={entry.at}
    >
      <FindingCard finding={entry.finding} showOpenLink />
    </EntryFrame>
  )
}
