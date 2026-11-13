import { Link } from "@tanstack/react-router"

import { RelativeTime } from "@/components/relative-time"
import { Cell } from "@/components/table-card"
import { dashboardLink } from "@/lib/links"

import type { HealthItem } from "./health-labels"
import { ProblemGroups } from "./problem-groups"
import { RepairControl } from "./repair-control"

type Props = {
  item: HealthItem
  agentAvailable: boolean
  pending: boolean
  onRepair: () => void
}

export function HealthRow({ item, agentAvailable, pending, onRepair }: Props) {
  return (
    <tr>
      <Cell className="align-top wrap-anywhere">
        <Link
          {...dashboardLink(item.id, item.title)}
          className="font-medium underline-offset-2 hover:underline"
        >
          {item.title}
        </Link>
        <span className="block text-xs font-normal text-muted-foreground">
          {item.folder.title}
        </span>
      </Cell>
      <Cell className="align-top">
        <ProblemGroups problems={item.problems} />
      </Cell>
      <Cell className="align-top whitespace-nowrap">
        <span className="hidden narrow:inline">Checked </span>
        <RelativeTime iso={item.checked_at} fallback="Not checked yet" />
        {item.skipped === "depends_on_viewer" ? (
          <span className="block text-xs text-muted-foreground narrow:inline narrow:before:content-['_·_']">
            Skipped: reads the viewer
          </span>
        ) : item.checked_version === null ? null : (
          <span className="block text-xs text-muted-foreground narrow:inline narrow:before:content-['_·_']">
            Version {item.checked_version}
          </span>
        )}
      </Cell>
      <Cell className="align-top whitespace-nowrap">
        <span className="hidden narrow:inline">Opened </span>
        <RelativeTime iso={item.last_viewed_at} fallback="Never" />
      </Cell>
      <Cell data-cell="action" className="text-right align-top">
        <RepairControl
          item={item}
          agentAvailable={agentAvailable}
          pending={pending}
          onRepair={onRepair}
          showOpen
        />
      </Cell>
    </tr>
  )
}
