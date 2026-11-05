import { useState } from "react"
import { useQuery } from "@tanstack/react-query"
import { toast } from "sonner"

import { LoadProblem } from "@/components/load-problem"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { useMoveDashboard } from "@/dashboards/queries"
import { foldersQuery } from "@/folders/queries"
import { loadProblemText } from "@/service-accounts/queries"

import { publishTargets } from "./publish"

export function PublishSection({
  dashboardId,
  headingId,
}: {
  dashboardId: string
  headingId: string
}) {
  const folders = useQuery(foldersQuery())
  const move = useMoveDashboard()
  const [chosen, setChosen] = useState<string>()

  const targets = folders.data === undefined ? [] : publishTargets(folders.data)
  const target = targets.find((folder) => folder.id === chosen) ?? targets.at(0)

  function publish() {
    if (target === undefined) return
    move.mutate(
      { id: dashboardId, folderId: target.id },
      {
        onSuccess: () => toast.success(`Published to ${target.title}`),
        onError: (error) => toast.error(loadProblemText(error)),
      }
    )
  }

  return (
    <section
      aria-labelledby={headingId}
      className="flex flex-col gap-2"
      aria-busy={folders.isPending}
    >
      <h3 id={headingId} className="text-[13px] font-semibold">
        Only you can see this dashboard
      </h3>
      {folders.isError ? (
        <LoadProblem
          title="Could not load your folders"
          detail={loadProblemText(folders.error)}
          retrying={folders.isFetching}
          onRetry={() => void folders.refetch()}
        />
      ) : folders.isPending ? (
        <Skeleton className="h-16 w-full animate-none motion-safe:animate-pulse" />
      ) : target === undefined ? (
        <p className="text-xs text-muted-foreground">
          You cannot edit any shared folder yet. A folder Admin can give you
          Edit on one.
        </p>
      ) : (
        <>
          <p className="text-xs text-muted-foreground">
            It is in your My Dashboards. Publish it to a shared folder so others
            can open it.
          </p>
          <div className="flex gap-1.5">
            <select
              aria-label="Folder to publish to"
              value={target.id}
              onChange={(event) => setChosen(event.target.value)}
              className="h-8 min-w-0 flex-1 rounded-control border border-input bg-background px-2 text-[13px] outline-none focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring narrow:text-base"
            >
              {targets.map((folder) => (
                <option key={folder.id} value={folder.id}>
                  {folder.title}
                </option>
              ))}
            </select>
            <Button
              disabled={move.isPending}
              aria-busy={move.isPending}
              onClick={publish}
            >
              Publish
            </Button>
          </div>
        </>
      )}
    </section>
  )
}
