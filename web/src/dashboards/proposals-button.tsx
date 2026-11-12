import { useQuery } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"

import { RelativeTime } from "@/components/relative-time"
import { Button } from "@/components/ui/button"
import {
  Popover,
  PopoverContent,
  PopoverTitle,
  PopoverTrigger,
} from "@/components/ui/popover"
import { proposalLink } from "@/lib/links"
import { proposalActor } from "@/proposals/proposal-view"
import { dashboardProposalsQuery } from "@/proposals/queries"

export function ProposalsButton({ dashboardId }: { dashboardId: string }) {
  const open = useQuery(dashboardProposalsQuery(dashboardId, "open")).data
  if (open === undefined || open.proposals.length === 0) return null
  const count = `${open.proposals.length}${open.next_cursor === null ? "" : "+"}`
  return (
    <Popover>
      <PopoverTrigger
        render={<Button variant="outline" size="sm" aria-haspopup="dialog" />}
      >
        {count} {open.proposals.length === 1 ? "proposal" : "proposals"}
      </PopoverTrigger>
      <PopoverContent align="end" className="w-80">
        <PopoverTitle>Proposals</PopoverTitle>
        <ul className="flex max-h-80 flex-col gap-2 overflow-auto">
          {open.proposals.map((proposal) => (
            <li key={proposal.id}>
              <Link
                {...proposalLink(proposal.id)}
                className="flex flex-col gap-0.5 rounded-control p-1.5 no-underline outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring"
              >
                <span className="text-[13px] font-medium wrap-anywhere">
                  {proposal.title}
                </span>
                <span className="flex flex-wrap items-center gap-x-1.5 text-xs text-muted-foreground">
                  <span>{proposalActor(proposal).label}</span>
                  <RelativeTime iso={proposal.created_at} />
                  {proposal.behind ? <span>Behind</span> : null}
                </span>
              </Link>
            </li>
          ))}
        </ul>
      </PopoverContent>
    </Popover>
  )
}
