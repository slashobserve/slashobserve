import { Link } from "@tanstack/react-router"

import { markOf } from "@/lib/author"
import { proposalLink } from "@/lib/links"

import { boxClass, EntryFrame } from "./entry-frame"
import type { TimelineEntry } from "./timeline-model"

type Props = {
  entry: Extract<TimelineEntry, { kind: "proposal" }>
}

export function ProposalEntry({ entry }: Props) {
  const { proposal } = entry
  return (
    <EntryFrame
      avatar={markOf(proposal.actor)}
      who={proposal.actor.name}
      verb="proposed a change"
      at={entry.at}
    >
      <Link
        {...proposalLink(proposal.id)}
        className={`${boxClass} flex items-center justify-between gap-2.5 no-underline outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring`}
      >
        <span className="min-w-0">
          <span className="font-medium wrap-anywhere">{proposal.title}</span>
          <span className="block text-xs text-muted-foreground">
            Open, waiting for a merge
          </span>
        </span>
        <span className="flex-none text-xs text-muted-foreground">Review</span>
      </Link>
    </EntryFrame>
  )
}
