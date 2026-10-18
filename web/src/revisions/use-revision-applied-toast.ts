import { useEffect } from "react"
import { toast } from "sonner"

import { markVersionApplied, versionToastId } from "./applied-toast"
import { useFollowUpStore } from "./follow-up-store"
import { isActive, useRevisionLookup } from "./queries"

const none: string[] = []

export function useRevisionAppliedToast(dashboardId: string, enabled: boolean) {
  const watched = useFollowUpStore(
    (state) => state.watched[dashboardId] ?? none
  )
  const unwatch = useFollowUpStore((state) => state.unwatch)
  const revisions = useRevisionLookup(dashboardId, watched)

  useEffect(() => {
    if (!enabled) return
    for (const revisionId of watched) {
      const revision = revisions.get(revisionId)
      if (revision === undefined || isActive(revision.status)) continue
      unwatch(dashboardId, revisionId)
      if (revision.status === "succeeded" && revision.result_version != null) {
        const id = versionToastId(dashboardId, revision.result_version)
        markVersionApplied(id)
        toast.success(
          `Revision applied as version ${revision.result_version}`,
          {
            id,
          }
        )
      }
    }
  }, [dashboardId, enabled, watched, revisions, unwatch])
}
