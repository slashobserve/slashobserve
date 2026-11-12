import type { Ref } from "react"

import { Button } from "@/components/ui/button"
import { changeRows } from "@/proposals/proposal-view"
import type { MergeConflict } from "@/proposals/queries"

type Props = {
  conflicts: MergeConflict[]
  titles: Map<string, string>
  latest: number
  onReload: () => void
  reloadRef?: Ref<HTMLButtonElement>
}

export function MergeConflicts({
  conflicts,
  titles,
  latest,
  onReload,
  reloadRef,
}: Props) {
  const rows = changeRows({ spec: {}, changes: [], conflicts }, titles)
  return (
    <div className="flex flex-col items-start gap-2">
      <p className="text-[13px] font-semibold">
        {rows.length > 0
          ? `Version ${latest} changed the same things`
          : `Someone saved version ${latest} since you started`}
      </p>
      {rows.length > 0 ? (
        <ul className="list-disc pl-4 text-[13px] wrap-anywhere">
          {rows.map((row) => (
            <li key={row.key}>
              {row.label}: {row.conflict?.message}
            </li>
          ))}
        </ul>
      ) : null}
      <Button ref={reloadRef} variant="outline" size="sm" onClick={onReload}>
        Reload version {latest}
      </Button>
    </div>
  )
}
