import { useQuery } from "@tanstack/react-query"
import { BookmarkIcon, LoaderCircleIcon, XIcon } from "lucide-react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import { folderLabel } from "@/saved-searches/folder-label"
import {
  matchesSaved,
  type LogsDraft,
  type SearchDraft,
} from "@/saved-searches/draft"
import {
  savedSearchesQuery,
  useUpdateSavedSearchContent,
  type SavedSearch,
} from "@/saved-searches/queries"
import { loadProblemText } from "@/service-accounts/queries"

import { useLogsWorkspace } from "./workspace"

export function OpenSavedSearch({ savable }: { savable: LogsDraft }) {
  const { datasource, search } = useLogsWorkspace()
  const listed = useQuery(savedSearchesQuery({ kind: "logs" }))
  const saved = listed.data?.saved_searches.find(
    (candidate) =>
      candidate.id === search.saved &&
      candidate.datasource_uid === datasource.uid
  )
  if (saved === undefined) return null
  const changedDraft =
    "draft" in savable && !matchesSaved(savable.draft, saved)
      ? savable.draft
      : undefined
  return <OpenSaved saved={saved} changedDraft={changedDraft} />
}

function OpenSaved({
  saved,
  changedDraft,
}: {
  saved: SavedSearch
  changedDraft: SearchDraft | undefined
}) {
  const { update } = useLogsWorkspace()
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
        onClick={() => update({ saved: undefined }, { replace: true })}
      >
        <XIcon aria-hidden="true" />
      </Button>
    </div>
  )
}
