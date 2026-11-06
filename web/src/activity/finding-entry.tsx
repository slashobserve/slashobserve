import { FindingCard } from "@/findings/finding-card"

import { EntryFrame } from "./entry-frame"
import type { TimelineEntry } from "./timeline-model"

type Props = {
  entry: Extract<TimelineEntry, { kind: "finding" }>
}

export function FindingEntry({ entry }: Props) {
  return (
    <EntryFrame
      avatar={{ kind: "agent" }}
      who="Agent"
      verb="found"
      at={entry.at}
    >
      <FindingCard finding={entry.finding} showOpenLink />
    </EntryFrame>
  )
}
