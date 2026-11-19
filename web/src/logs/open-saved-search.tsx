import { useQuery } from "@tanstack/react-query"
import { BookmarkIcon, XIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { folderLabel } from "@/saved-searches/folder-label"
import { matchesSaved, type LogsDraft } from "@/saved-searches/draft"
import { savedSearchesQuery } from "@/saved-searches/queries"

import { useLogsWorkspace } from "./workspace"

export function OpenSavedSearch({ savable }: { savable: LogsDraft }) {
  const { datasource, search, update } = useLogsWorkspace()
  const listed = useQuery(savedSearchesQuery({ kind: "logs" }))
  const saved = listed.data?.saved_searches.find(
    (candidate) =>
      candidate.id === search.saved &&
      candidate.datasource_uid === datasource.uid
  )
  if (saved === undefined) return null
  const changed = "draft" in savable && !matchesSaved(savable.draft, saved)

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
      {changed ? (
        <span>
          You changed it. Save search saves a new search; this one stays as it
          was saved.
        </span>
      ) : null}
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
