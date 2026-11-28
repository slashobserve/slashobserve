import { useQuery } from "@tanstack/react-query"
import { BookmarkIcon, LoaderCircleIcon, XIcon } from "lucide-react"
import { toast } from "sonner"

import { loadProblemText } from "@/api/errors"
import { Button } from "@/components/ui/button"

import { matchesSaved, type SearchDraft } from "./draft"
import { folderLabel } from "./folder-label"
import {
  savedSearchesQuery,
  useUpdateSavedSearchContent,
  type SavedSearch,
} from "./queries"

export function OpenSavedSearch({
  kind,
  uid,
  savedId,
  draft,
  onClose,
}: {
  kind: SavedSearch["kind"]
  uid: string
  savedId: string | undefined
  draft: SearchDraft | undefined
  onClose: () => void
}) {
  const listed = useQuery(savedSearchesQuery({ kind }))
  const saved = listed.data?.saved_searches.find(
    (candidate) => candidate.id === savedId && candidate.datasource_uid === uid
  )
  if (saved === undefined) return null
  const changedDraft =
    draft !== undefined && !matchesSaved(draft, saved) ? draft : undefined
  return (
    <OpenSaved saved={saved} changedDraft={changedDraft} onClose={onClose} />
  )
}

function OpenSaved({
  saved,
  changedDraft,
  onClose,
}: {
  saved: SavedSearch
  changedDraft: SearchDraft | undefined
  onClose: () => void
}) {
  const change = useUpdateSavedSearchContent(saved.id)
  const mayUpdate = saved.permissions.edit

  function updateSaved(draft: SearchDraft) {
    change.mutate(draft, {
      onSuccess: () => toast.success(`Updated ${saved.title}`),
      onError: (error) => toast.error(loadProblemText(error)),
    })
  }

  return (
    <div
      role="status"
      className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground"
    >
      <BookmarkIcon aria-hidden="true" className="size-3.5 shrink-0" />
      <span className="min-w-0 truncate">
        Saved search{" "}
        <span className="font-medium text-foreground">{saved.title}</span> in{" "}
        {folderLabel(saved.folder)}
      </span>
      {changedDraft === undefined ? null : (
        <>
          <span>
            {mayUpdate
              ? "You changed it. Update saved search saves the change into it; Save search saves a new search."
              : "You changed it. Save search saves a new search; this one stays as it was saved."}
          </span>
          {mayUpdate ? (
            <Button
              type="button"
              variant="outline"
              size="xs"
              disabled={change.isPending}
              onClick={() => updateSaved(changedDraft)}
            >
              {change.isPending ? (
                <LoaderCircleIcon aria-hidden="true" className="animate-spin" />
              ) : null}
              Update saved search
            </Button>
          ) : null}
        </>
      )}
      <Button
        type="button"
        variant="ghost"
        size="xs"
        aria-label="Close the saved search"
        onClick={onClose}
      >
        <XIcon aria-hidden="true" />
      </Button>
    </div>
  )
}
