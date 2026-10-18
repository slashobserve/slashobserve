import { useCallback, useId, useState, type ReactNode } from "react"

import { Checkbox } from "@/components/ui/checkbox"
import { Label } from "@/components/ui/label"
import type { Comment } from "@/comments/queries"

import { useAgentAvailable, useRevisionById } from "./queries"
import { RevisionStatus } from "./revision-status"

function RevisionCheckbox({
  checked,
  onCheckedChange,
}: {
  checked: boolean
  onCheckedChange: (checked: boolean) => void
}) {
  const id = useId()
  return (
    <Label htmlFor={id} className="w-fit cursor-pointer leading-snug">
      <Checkbox id={id} checked={checked} onCheckedChange={onCheckedChange} />
      Ask the agent to revise this panel
    </Label>
  )
}

export function useCommentRevision(dashboardId: string): {
  requestRevision: boolean
  composerExtraFields: ReactNode
  renderRevision: (comment: Comment) => ReactNode
} {
  const available = useAgentAvailable()
  const [asked, setAsked] = useState(true)
  const byId = useRevisionById(dashboardId)

  const renderRevision = useCallback(
    (comment: Comment) =>
      comment.revision === null ? null : (
        <RevisionStatus
          revision={byId.get(comment.revision.id) ?? comment.revision}
          dashboardId={dashboardId}
        />
      ),
    [byId, dashboardId]
  )

  return {
    requestRevision: available && asked,
    composerExtraFields: available ? (
      <RevisionCheckbox checked={asked} onCheckedChange={setAsked} />
    ) : null,
    renderRevision,
  }
}
