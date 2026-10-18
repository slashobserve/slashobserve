import { useCallback, useId, type ReactNode } from "react"

import { Checkbox } from "@/components/ui/checkbox"
import { Label } from "@/components/ui/label"
import { cn } from "@/lib/utils"
import type { Comment } from "@/comments/queries"

import { useCommentRevisionPreference } from "./comment-preference"
import { useFollowUpStore } from "./follow-up-store"
import { useAgentAvailable, useRevisionLookup } from "./queries"
import { RevisionStatus } from "./revision-status"

function RevisionCheckbox({
  checked,
  disabled,
  reason,
  onCheckedChange,
}: {
  checked: boolean
  disabled: boolean
  reason: string | null
  onCheckedChange: (checked: boolean) => void
}) {
  const id = useId()
  const reasonId = useId()
  return (
    <div className="flex flex-col gap-1">
      <Label
        htmlFor={id}
        className={cn(
          "w-fit leading-snug",
          disabled ? "text-muted-foreground" : "cursor-pointer"
        )}
      >
        <Checkbox
          id={id}
          checked={checked}
          disabled={disabled}
          aria-describedby={reason === null ? undefined : reasonId}
          onCheckedChange={onCheckedChange}
        />
        Ask the agent to revise this panel
      </Label>
      {reason === null ? null : (
        <p
          id={reasonId}
          className="pl-6 text-xs text-pretty text-muted-foreground"
        >
          {reason}
        </p>
      )}
    </div>
  )
}

function CommentRevisionStatus({
  comment,
  dashboardId,
}: {
  comment: Comment
  dashboardId: string
}) {
  const revision = comment.revision
  const revisions = useRevisionLookup(
    dashboardId,
    revision === null ? [] : [revision.id]
  )
  if (revision === null) return null
  return (
    <RevisionStatus
      revision={revisions.get(revision.id) ?? revision}
      dashboardId={dashboardId}
    />
  )
}

export function useCommentRevision(
  dashboardId: string,
  version: number,
  latestVersion: number
): {
  requestRevision: boolean
  composerExtraFields: ReactNode
  renderRevision: (comment: Comment) => ReactNode
  onCommentCreated: (comment: Comment) => void
} {
  const available = useAgentAvailable()
  const asked = useCommentRevisionPreference((state) => state.asked)
  const setAsked = useCommentRevisionPreference((state) => state.setAsked)
  const watch = useFollowUpStore((state) => state.watch)
  const onOldVersion = version < latestVersion

  const renderRevision = useCallback(
    (comment: Comment) =>
      comment.revision === null ? null : (
        <CommentRevisionStatus comment={comment} dashboardId={dashboardId} />
      ),
    [dashboardId]
  )
  const onCommentCreated = useCallback(
    (comment: Comment) => {
      if (comment.revision !== null) watch(dashboardId, comment.revision.id)
    },
    [dashboardId, watch]
  )

  return {
    requestRevision: available && asked && !onOldVersion,
    composerExtraFields: available ? (
      <RevisionCheckbox
        checked={asked && !onOldVersion}
        disabled={onOldVersion}
        reason={
          onOldVersion
            ? `Only the latest version (v${latestVersion}) can be revised. Your comment is still posted.`
            : null
        }
        onCheckedChange={setAsked}
      />
    ) : null,
    renderRevision,
    onCommentCreated,
  }
}
