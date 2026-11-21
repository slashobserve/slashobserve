import { useRef, useState } from "react"
import { useQuery, useQueryClient } from "@tanstack/react-query"

import { Button } from "@/components/ui/button"
import { foldersQuery } from "@/folders/queries"

import { duplicateTargets } from "./duplicate"
import { DuplicateDashboardDialog } from "./duplicate-dashboard-dialog"
import { latestDashboardQuery } from "./queries"

export function DuplicateAction({ dashboardId }: { dashboardId: string }) {
  const queryClient = useQueryClient()
  const latest = useQuery({
    ...latestDashboardQuery(queryClient, dashboardId),
    staleTime: Infinity,
  }).data
  const folders = useQuery(foldersQuery()).data ?? []
  const [open, setOpen] = useState(false)
  const trigger = useRef<HTMLButtonElement>(null)
  if (latest === undefined) return null
  const { dashboard } = latest
  const targets = duplicateTargets(folders, dashboard.folder.id)
  if (targets.choices.length === 0) return null
  return (
    <>
      <Button
        ref={trigger}
        type="button"
        variant="outline"
        size="sm"
        onClick={() => setOpen(true)}
      >
        Duplicate
      </Button>
      <DuplicateDashboardDialog
        source={{
          id: dashboard.id,
          title: dashboard.title,
          version: dashboard.latest_version,
        }}
        targets={targets}
        open={open}
        onOpenChange={setOpen}
        finalFocus={() => trigger.current ?? true}
      />
    </>
  )
}
