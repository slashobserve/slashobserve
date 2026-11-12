import { useId, useState, type RefObject } from "react"
import { useQuery } from "@tanstack/react-query"
import { LoaderCircleIcon } from "lucide-react"
import { toast } from "sonner"

import { LoadProblem } from "@/components/load-problem"
import { RelativeTime } from "@/components/relative-time"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import type { Observation } from "@/observations/queries"
import { cn } from "@/lib/utils"
import { loadProblemText } from "@/service-accounts/queries"
import { SidePanel } from "@/shell/side-panel"

import { publishProblem } from "./publish-problem"
import { RcaLinks } from "./rca-links"
import { rcasQuery, usePublishRca, type RcaSummary } from "./queries"

type Props = {
  observation: Observation
  onClose: () => void
  trigger: RefObject<HTMLElement | null>
}

export function RcaPanel({ observation, onClose, trigger }: Props) {
  const headingId = useId()
  const revisions = useQuery(rcasQuery(observation.id))
  const publish = usePublishRca(observation.id)
  const [selected, setSelected] = useState<number | null>(null)
  const [problem, setProblem] = useState<string | null>(null)
  const canPublish = observation.allowed_actions.includes("publish")
  const list = revisions.data
  const next = (list?.[0]?.revision ?? 0) + 1

  function submit() {
    if (publish.isPending) return
    setProblem(null)
    publish.mutate(undefined, {
      onSuccess: ({ rca }) => {
        toast.success(`Published revision ${rca.revision}`)
        setSelected(rca.revision)
      },
      onError: (error) => setProblem(publishProblem(error)),
    })
  }

  return (
    <SidePanel
      title="Publish as RCA"
      description="Freezes every live panel and saves a numbered revision. Earlier revisions do not change."
      trigger={trigger}
      closeDisabled={publish.isPending}
      onClose={onClose}
    >
      {canPublish ? (
        list === undefined ? null : (
          <div className="flex flex-col gap-2">
            <div>
              <Button
                focusableWhenDisabled
                disabled={publish.isPending}
                aria-busy={publish.isPending}
                onClick={submit}
              >
                {publish.isPending ? (
                  <LoaderCircleIcon
                    aria-hidden="true"
                    className="size-3.5 motion-safe:animate-spin"
                  />
                ) : null}
                {publish.isPending ? "Publishing" : `Publish revision ${next}`}
              </Button>
            </div>
            {problem === null ? null : (
              <Alert variant="destructive">
                <AlertDescription>{problem}</AlertDescription>
              </Alert>
            )}
          </div>
        )
      ) : (
        <p className="text-[13px] text-muted-foreground">
          You can view revisions. Editors publish them.
        </p>
      )}
      <section aria-labelledby={headingId} className="flex flex-col gap-2">
        <h3 id={headingId} className="text-[13px] font-semibold">
          Revisions
        </h3>
        {list !== undefined ? (
          list.length === 0 ? (
            <p className="text-[13px] text-muted-foreground">
              Not published yet.
            </p>
          ) : (
            <ul className="rounded-card border">
              {list.map((rca) => (
                <RevisionRow
                  key={rca.id}
                  rca={rca}
                  selected={selected === rca.revision}
                  onSelect={() =>
                    setSelected(selected === rca.revision ? null : rca.revision)
                  }
                />
              ))}
            </ul>
          )
        ) : revisions.isError ? (
          <LoadProblem
            title="Could not load revisions"
            detail={loadProblemText(revisions.error)}
            retrying={revisions.isFetching}
            onRetry={() => void revisions.refetch()}
          />
        ) : (
          <Skeleton className="h-16 w-full animate-none motion-safe:animate-pulse" />
        )}
      </section>
      {selected === null ? null : (
        <section
          aria-label={`Public links of revision ${selected}`}
          className="flex flex-col gap-2"
        >
          <h3 className="text-[13px] font-semibold">
            Public links, revision {selected}
          </h3>
          <p className="text-xs text-muted-foreground">
            Anyone with a link can view this frozen revision without signing in.
            It never shows a query.
          </p>
          <RcaLinks
            key={selected}
            observationId={observation.id}
            revision={selected}
          />
        </section>
      )}
    </SidePanel>
  )
}

function RevisionRow({
  rca,
  selected,
  onSelect,
}: {
  rca: RcaSummary
  selected: boolean
  onSelect: () => void
}) {
  return (
    <li className="border-b last:border-b-0">
      <button
        type="button"
        aria-pressed={selected}
        onClick={onSelect}
        className={cn(
          "flex w-full items-baseline justify-between gap-3 px-3 py-2 text-left text-[13px] outline-none first:rounded-t-card last:rounded-b-card hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring",
          selected && "bg-muted"
        )}
      >
        <span className="font-medium">Revision {rca.revision}</span>
        <span className="min-w-0 truncate text-xs text-muted-foreground">
          {rca.created_by.label}, <RelativeTime iso={rca.created_at} />
        </span>
      </button>
    </li>
  )
}
