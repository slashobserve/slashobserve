import { useEffect, useRef, useState } from "react"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import { useRouteContext } from "@tanstack/react-router"
import { LoaderCircleIcon } from "lucide-react"
import { toast } from "sonner"

import { dashboardKeys } from "@/api/keys"
import { Button } from "@/components/ui/button"
import { roleAtLeast } from "@/lib/roles"
import { loadProblemText } from "@/service-accounts/queries"

import { lastRunProblem, summaryLabels, type RunRequest } from "./health-labels"
import { healthSummaryQuery, useRunHealth } from "./queries"
import { SummaryLine } from "./summary-line"

export function ListHealthHeader({ folderId }: { folderId: string | null }) {
  const { principal } = useRouteContext({ from: "/_authed" })
  const isAdmin = roleAtLeast(principal.role, "admin")
  const queryClient = useQueryClient()
  const [run, setRun] = useState<RunRequest | null>(null)
  const health = useQuery(healthSummaryQuery(folderId, run)).data
  const runHealth = useRunHealth()
  const job = health?.job
  const finishedAt = job?.last_finished_at
  const seenFinish = useRef<string | null | undefined>(undefined)

  useEffect(() => {
    if (finishedAt === undefined) return
    const previous = seenFinish.current
    seenFinish.current = finishedAt
    if (previous !== undefined && previous !== finishedAt) {
      void queryClient.invalidateQueries({ queryKey: dashboardKeys.list() })
    }
  }, [finishedAt, queryClient])

  if (health === undefined || job === undefined) return null
  const hasNews =
    (health.summary !== null && summaryLabels(health.summary).length > 0) ||
    job.running ||
    lastRunProblem(job) !== null
  if (!hasNews && !isAdmin) return null

  function startRun() {
    const previousStart = job?.last_started_at ?? null
    runHealth.mutate(undefined, {
      onSuccess: () => {
        setRun({ previousStart, at: Date.now() })
        toast.success("Health run started")
      },
      onError: (error) => toast.error(loadProblemText(error)),
    })
  }

  return (
    <SummaryLine
      summary={health.summary}
      job={job}
      action={
        isAdmin ? (
          <Button
            variant="ghost"
            size="xs"
            disabled={job.running || runHealth.isPending}
            focusableWhenDisabled
            aria-busy={runHealth.isPending}
            onClick={startRun}
          >
            {runHealth.isPending ? (
              <LoaderCircleIcon
                aria-hidden="true"
                className="motion-safe:animate-spin"
              />
            ) : null}
            Run now
          </Button>
        ) : null
      }
    />
  )
}
