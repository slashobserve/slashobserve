import { useMemo } from "react"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"

import { AuthorMark } from "@/components/author-mark"
import { RelativeTime } from "@/components/relative-time"
import { latestDashboardQuery } from "@/dashboards/queries"
import { panelTitles } from "@/history/diff-groups"
import { markOf } from "@/lib/author"
import { dashboardLink } from "@/lib/links"

import { ConflictChoices, type Side } from "./conflict-choices"
import {
  changeRows,
  stateLine,
  type ChangeRow,
  type Picks,
} from "./proposal-view"
import type { Proposal } from "./queries"

type Props = {
  proposal: Proposal
  picks: Picks
  onPick: (key: string, side: Side) => void
  disabled?: boolean
  headingLevel?: 1 | 2
}

function Row({
  row,
  picks,
  onPick,
  disabled,
}: {
  row: ChangeRow
  picks: Picks
  onPick: Props["onPick"]
  disabled: boolean
}) {
  return (
    <li className="flex flex-col gap-2 py-3">
      <div className="flex items-center gap-2">
        <span className="text-[13px] font-semibold wrap-anywhere">
          {row.label}
        </span>
        <span className="rounded-control bg-muted px-2 py-0.5 text-xs">
          {row.change}
        </span>
      </div>
      {row.conflict === null ? null : (
        <ConflictChoices
          conflict={row.conflict}
          label={row.label}
          value={picks[row.key]}
          disabled={disabled}
          onChange={(side) => onPick(row.key, side)}
        />
      )}
    </li>
  )
}

export function ProposalBody({
  proposal,
  picks,
  onPick,
  disabled = false,
  headingLevel = 1,
}: Props) {
  const Heading = headingLevel === 1 ? "h1" : "h2"
  const SectionHeading = headingLevel === 1 ? "h2" : "h3"
  const queryClient = useQueryClient()
  const latest = useQuery(
    latestDashboardQuery(queryClient, proposal.dashboard_id)
  )
  const latestSpec = latest.data?.version.spec
  const rows = useMemo(
    () => changeRows(proposal, panelTitles(latestSpec)),
    [proposal, latestSpec]
  )
  const { actor } = proposal
  return (
    <div className="flex flex-col gap-4">
      <Heading className="text-xl font-semibold wrap-anywhere">
        {proposal.title}
      </Heading>
      <div className="flex flex-col gap-1.5">
        <div className="flex items-center gap-2 text-[13px]">
          <AuthorMark mark={markOf(actor)} size={24} />
          <span className="font-medium">{actor.label}</span>
          <span className="text-muted-foreground">
            <RelativeTime iso={proposal.created_at} />
          </span>
        </div>
        <p className="text-[13px] text-muted-foreground">
          {stateLine(proposal)}
        </p>
      </div>
      {proposal.summary === "" ? null : (
        <p className="text-sm wrap-anywhere">{proposal.summary}</p>
      )}
      <section className="flex flex-col gap-1">
        <SectionHeading className="text-[13px] font-medium">
          Changes
        </SectionHeading>
        {rows.length === 0 ? (
          <p className="text-[13px] text-muted-foreground">
            This proposal changes nothing on the current dashboard.
          </p>
        ) : (
          <ul className="divide-y divide-border">
            {rows.map((row) => (
              <Row
                key={row.key}
                row={row}
                picks={picks}
                onPick={onPick}
                disabled={disabled}
              />
            ))}
          </ul>
        )}
      </section>
      <Link
        {...dashboardLink(proposal.dashboard_id)}
        search={{ version: proposal.base_version }}
        className="w-fit text-[13px] underline underline-offset-4"
      >
        Open the dashboard at version {proposal.base_version}
      </Link>
    </div>
  )
}
