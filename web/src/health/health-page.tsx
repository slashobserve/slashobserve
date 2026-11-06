import { useId, useState } from "react"
import { useInfiniteQuery, useQuery } from "@tanstack/react-query"
import { Link, useRouteContext } from "@tanstack/react-router"
import { LoaderCircleIcon } from "lucide-react"
import { toast } from "sonner"

import { LoadProblem } from "@/components/load-problem"
import { PageHeader } from "@/components/page-header"
import { RelativeTime } from "@/components/relative-time"
import { StatusLabel } from "@/components/status-label"
import { Cell, HeaderCell, TableCard } from "@/components/table-card"
import { Button } from "@/components/ui/button"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { foldersQuery } from "@/folders/queries"
import { dashboardLink } from "@/lib/links"
import { roleAtLeast } from "@/lib/roles"
import { useAgentInfo } from "@/observe/queries"
import { loadProblemText } from "@/service-accounts/queries"

import {
  healthKinds,
  jobLine,
  kindLabels,
  kindTones,
  pollHealth,
  problemSubject,
  repairOutcomeText,
  repairReason,
  sortProblems,
  type HealthItem,
  type HealthJob,
  type HealthKind,
  type RunRequest,
} from "./health-labels"
import { healthQuery, useRequestHealthRepair, useRunHealth } from "./queries"

type Props = {
  kind: HealthKind | null
  folderId: string | null
  onFilters: (filters: {
    kind: HealthKind | null
    folderId: string | null
  }) => void
}

const pollMs = 5000
const allValue = "all"

export function HealthPage({ kind, folderId, onFilters }: Props) {
  const { principal } = useRouteContext({ from: "/_authed" })
  const isAdmin = roleAtLeast(principal.role, "admin")
  const agent = useAgentInfo()
  const agentAvailable = agent.data?.available ?? false
  const [run, setRun] = useState<RunRequest | null>(null)
  const list = useInfiniteQuery({
    ...healthQuery(kind, folderId),
    refetchInterval: (query) =>
      pollHealth(query.state.data?.pages[0]?.job, run, Date.now())
        ? pollMs
        : false,
  })
  const repair = useRequestHealthRepair()
  const runHealth = useRunHealth()
  const items = list.data?.pages.flatMap((page) => page.dashboards) ?? []
  const job = list.data?.pages[0]?.job
  const pendingId = repair.isPending ? repair.variables : null

  function requestRepair(dashboardId: string) {
    repair.mutate(dashboardId, {
      onSuccess: (outcome) => toast.success(repairOutcomeText(outcome)),
      onError: (error) => toast.error(loadProblemText(error)),
    })
  }

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
    <>
      <PageHeader
        title="Dashboard health"
        description="Dashboards with panels that fail or return no data, variables that resolve to nothing, duplicate titles in a folder, or nobody opening them for 90 days. Every dashboard is checked about once a day."
        actions={
          <>
            <KindSelect
              kind={kind}
              onKind={(next) => onFilters({ kind: next, folderId })}
            />
            <FolderSelect
              folderId={folderId}
              onFolder={(next) => onFilters({ kind, folderId: next })}
            />
            {isAdmin ? (
              <Button
                variant="outline"
                disabled={
                  job === undefined || job.running || runHealth.isPending
                }
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
            ) : null}
          </>
        }
      />
      {job === undefined ? null : <JobStatus job={job} />}
      {list.isError && list.data === undefined ? (
        <LoadProblem
          title="Could not load dashboard health"
          detail={loadProblemText(list.error)}
          retrying={list.isFetching}
          onRetry={() => void list.refetch()}
        />
      ) : list.data === undefined ? (
        <Skeleton className="h-32 w-full" />
      ) : items.length === 0 ? (
        <p className="rounded-card border bg-background px-4 py-6 text-[13px] text-muted-foreground">
          {kind === null && folderId === null
            ? "No dashboard has a problem."
            : "No dashboard matches these filters."}
        </p>
      ) : (
        <>
          <HealthTable
            items={items}
            agentAvailable={agentAvailable}
            pendingId={pendingId}
            onRepair={requestRepair}
          />
          {list.hasNextPage ? (
            <div className="mt-3 flex justify-center">
              <Button
                variant="outline"
                disabled={list.isFetchingNextPage}
                aria-busy={list.isFetchingNextPage}
                onClick={() => void list.fetchNextPage()}
              >
                Show more
              </Button>
            </div>
          ) : null}
        </>
      )}
    </>
  )
}

function JobStatus({ job }: { job: HealthJob }) {
  const line = jobLine(job)
  return (
    <p
      className="mb-4 flex items-center gap-1.5 text-[13px] text-muted-foreground"
      aria-live="polite"
    >
      <StatusLabel tone={line.tone}>{line.label}</StatusLabel>
      {line.at === null ? null : <RelativeTime iso={line.at} />}
    </p>
  )
}

function KindSelect({
  kind,
  onKind,
}: {
  kind: HealthKind | null
  onKind: (kind: HealthKind | null) => void
}) {
  const items = [
    { value: allValue, label: "All problems" },
    ...healthKinds.map((k) => ({ value: k, label: kindLabels[k] })),
  ]
  return (
    <Select
      items={items}
      value={kind ?? allValue}
      onValueChange={(next) =>
        onKind(next === null || next === allValue ? null : (next as HealthKind))
      }
    >
      <SelectTrigger aria-label="Problem" className="w-56">
        <SelectValue />
      </SelectTrigger>
      <SelectContent alignItemWithTrigger={false} align="end">
        {items.map((item) => (
          <SelectItem key={item.value} value={item.value}>
            {item.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

function FolderSelect({
  folderId,
  onFolder,
}: {
  folderId: string | null
  onFolder: (folderId: string | null) => void
}) {
  const folders = useQuery(foldersQuery())
  const items = [
    { value: allValue, label: "All folders" },
    ...(folders.data ?? []).map((folder) => ({
      value: folder.id,
      label: folder.title,
    })),
  ]
  return (
    <Select
      items={items}
      value={folderId ?? allValue}
      onValueChange={(next) =>
        onFolder(next === null || next === allValue ? null : next)
      }
    >
      <SelectTrigger aria-label="Folder" className="w-56">
        <SelectValue />
      </SelectTrigger>
      <SelectContent alignItemWithTrigger={false} align="end">
        {items.map((item) => (
          <SelectItem key={item.value} value={item.value}>
            {item.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

function HealthTable({
  items,
  agentAvailable,
  pendingId,
  onRepair,
}: {
  items: HealthItem[]
  agentAvailable: boolean
  pendingId: string | null
  onRepair: (dashboardId: string) => void
}) {
  return (
    <TableCard label="Dashboards with problems" minWidth={960}>
      <thead>
        <tr>
          <HeaderCell>Dashboard</HeaderCell>
          <HeaderCell>Folder</HeaderCell>
          <HeaderCell>Problems</HeaderCell>
          <HeaderCell>Checked</HeaderCell>
          <HeaderCell>Last opened</HeaderCell>
          <HeaderCell>
            <span className="sr-only">Actions</span>
          </HeaderCell>
        </tr>
      </thead>
      <tbody>
        {items.map((item) => (
          <HealthRow
            key={item.id}
            item={item}
            agentAvailable={agentAvailable}
            pending={pendingId === item.id}
            onRepair={() => onRepair(item.id)}
          />
        ))}
      </tbody>
    </TableCard>
  )
}

function HealthRow({
  item,
  agentAvailable,
  pending,
  onRepair,
}: {
  item: HealthItem
  agentAvailable: boolean
  pending: boolean
  onRepair: () => void
}) {
  const reasonId = useId()
  const reason = repairReason(item, agentAvailable)
  return (
    <tr className="align-top">
      <Cell className="wrap-anywhere">
        <Link
          {...dashboardLink(item.id, item.title)}
          className="font-medium underline-offset-2 hover:underline"
        >
          {item.title}
        </Link>
      </Cell>
      <Cell className="wrap-anywhere">{item.folder.title}</Cell>
      <Cell>
        <ul className="flex flex-col gap-1.5">
          {sortProblems(item.problems).map((problem, index) => (
            <li key={index} className="flex flex-col gap-0.5">
              <span className="flex flex-wrap items-center gap-x-1.5">
                <StatusLabel tone={kindTones[problem.kind]}>
                  {kindLabels[problem.kind]}
                </StatusLabel>
                {problemSubject(problem) === "" ? null : (
                  <span className="wrap-anywhere">
                    {problemSubject(problem)}
                  </span>
                )}
              </span>
              {problem.message === null ? null : (
                <span className="text-xs wrap-anywhere text-muted-foreground">
                  {problem.message}
                </span>
              )}
            </li>
          ))}
        </ul>
      </Cell>
      <Cell className="whitespace-nowrap">
        <RelativeTime iso={item.checked_at} fallback="Not checked yet" />
        {item.skipped === "depends_on_viewer" ? (
          <span className="block text-xs text-muted-foreground">
            Skipped: reads the viewer
          </span>
        ) : item.checked_version === null ? null : (
          <span className="block text-xs text-muted-foreground">
            Version {item.checked_version}
          </span>
        )}
      </Cell>
      <Cell className="whitespace-nowrap">
        <RelativeTime iso={item.last_viewed_at} />
      </Cell>
      <Cell className="text-right">
        {item.can_edit ? (
          <>
            <Button
              variant="outline"
              size="sm"
              disabled={reason !== null || pending}
              focusableWhenDisabled
              aria-busy={pending}
              aria-describedby={reason === null ? undefined : reasonId}
              title={reason ?? undefined}
              onClick={onRepair}
            >
              {pending ? (
                <LoaderCircleIcon
                  aria-hidden="true"
                  className="motion-safe:animate-spin"
                />
              ) : null}
              Repair
            </Button>
            {reason === null ? null : (
              <span id={reasonId} className="sr-only">
                {reason}
              </span>
            )}
          </>
        ) : null}
      </Cell>
    </tr>
  )
}
