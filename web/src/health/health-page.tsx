import { useState } from "react"
import { useInfiniteQuery, useQuery } from "@tanstack/react-query"
import { useRouteContext } from "@tanstack/react-router"
import { LoaderCircleIcon } from "lucide-react"
import { toast } from "sonner"

import { EmptyState } from "@/components/empty-state"
import { FilterToolbar } from "@/components/filter-toolbar"
import { LoadProblem } from "@/components/load-problem"
import { PageHeader } from "@/components/page-header"
import { RelativeTime } from "@/components/relative-time"
import { RoleGate } from "@/components/role-gate"
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
import { RepairQueue } from "@/import/repair/repair-page"
import { roleAtLeast } from "@/lib/roles"
import { useAgentInfo } from "@/observe/queries"
import { loadProblemText } from "@/service-accounts/queries"

import {
  dashboardCount,
  healthKinds,
  importedFilter,
  importedLabel,
  kindLabels,
  lastRunProblem,
  pollHealth,
  repairOutcomeText,
  summaryLabels,
  type HealthFilter,
  type HealthItem,
  type HealthJob,
  type HealthSummary,
  type RunRequest,
} from "./health-labels"
import { HealthRow } from "./health-row"
import { healthQuery, useRequestHealthRepair, useRunHealth } from "./queries"

type Filters = { kind: HealthFilter | null; folderId: string | null }

type Props = Filters & {
  onFilters: (filters: Filters) => void
}

const pollMs = 5000
const allValue = "all"
const skeletonRows = 3

export function HealthPage({ kind, folderId, onFilters }: Props) {
  const { principal } = useRouteContext({ from: "/_authed" })
  const isAdmin = roleAtLeast(principal.role, "admin")
  const isEditor = roleAtLeast(principal.role, "editor")
  const imported = kind === importedFilter
  const healthKind = kind === importedFilter ? null : kind
  const agent = useAgentInfo()
  const agentAvailable = agent.data?.available ?? false
  const [run, setRun] = useState<RunRequest | null>(null)
  const list = useInfiniteQuery({
    ...healthQuery(healthKind, folderId),
    refetchInterval: (query) => {
      const pages = query.state.data?.pages ?? []
      return pollHealth(
        pages[0]?.job,
        pages.flatMap((page) => page.dashboards),
        run,
        Date.now()
      )
        ? pollMs
        : false
    },
  })
  const repair = useRequestHealthRepair()
  const runHealth = useRunHealth()
  const items = list.data?.pages.flatMap((page) => page.dashboards) ?? []
  const job = list.data?.pages[0]?.job
  const summary = list.data?.pages[0]?.summary ?? null
  const pendingId = repair.isPending ? repair.variables : null
  const filtered = kind !== null || folderId !== null
  const allClear = list.data !== undefined && !filtered && items.length === 0

  function clearFilters() {
    onFilters({ kind: null, folderId: null })
  }

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
        description="Dashboards whose panels fail, return no data, or nobody opens. Checked about once a day."
        actions={
          isAdmin ? (
            <Button
              variant="outline"
              disabled={job === undefined || job.running || runHealth.isPending}
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
      {job === undefined || allClear ? null : (
        <SummaryLine summary={summary} job={job} />
      )}
      <FilterToolbar
        label="Health filters"
        onClear={filtered ? clearFilters : undefined}
        summary={
          imported || summary === null
            ? undefined
            : dashboardCount(summary, healthKind)
        }
      >
        <KindSelect
          kind={kind}
          offerImported={isEditor || imported}
          onKind={(next) => onFilters({ kind: next, folderId })}
        />
        <FolderSelect
          folderId={folderId}
          onFolder={(next) => onFilters({ kind, folderId: next })}
        />
      </FilterToolbar>
      {imported ? (
        <RoleGate
          min="editor"
          note="Only Editors can see the imported dashboards that are not verified."
        >
          <RepairQueue folderId={folderId} />
        </RoleGate>
      ) : list.isError && list.data === undefined ? (
        <LoadProblem
          title="Could not load dashboard health"
          detail={loadProblemText(list.error)}
          retrying={list.isFetching}
          onRetry={() => void list.refetch()}
        />
      ) : list.data === undefined ? (
        <HealthSkeleton />
      ) : allClear ? (
        <EmptyState tone="ok" title="Every dashboard passed its last check.">
          {job?.last_finished_at ? (
            <>
              Checked <RelativeTime iso={job.last_finished_at} />.
            </>
          ) : (
            "No check has finished yet."
          )}
        </EmptyState>
      ) : items.length === 0 ? (
        <div className="rounded-card border bg-background">
          <EmptyState
            title="No dashboard matches these filters."
            action={
              <Button variant="ghost" size="sm" onClick={clearFilters}>
                Clear filters
              </Button>
            }
          />
        </div>
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

function SummaryLine({
  summary,
  job,
}: {
  summary: HealthSummary | null
  job: HealthJob
}) {
  const runProblem = lastRunProblem(job)
  return (
    <div
      role="status"
      className="mb-3.5 flex flex-wrap items-center gap-x-5 gap-y-2 text-[13px]"
    >
      {(summary === null ? [] : summaryLabels(summary)).map((label) => (
        <span key={label.label} className="font-medium">
          <StatusLabel tone={label.tone}>{label.label}</StatusLabel>
        </span>
      ))}
      <span className="ml-auto flex flex-wrap items-center gap-x-3 text-xs text-muted-foreground">
        {runProblem === null ? null : (
          <StatusLabel tone={runProblem.tone}>{runProblem.label}</StatusLabel>
        )}
        {job.running ? (
          <StatusLabel tone="running">Checking now</StatusLabel>
        ) : job.last_finished_at === null ? (
          <span>No check has finished yet</span>
        ) : (
          <span>
            Last checked <RelativeTime iso={job.last_finished_at} />
          </span>
        )}
      </span>
    </div>
  )
}

function KindSelect({
  kind,
  offerImported,
  onKind,
}: {
  kind: HealthFilter | null
  offerImported: boolean
  onKind: (kind: HealthFilter | null) => void
}) {
  const items = [
    { value: allValue, label: "All problems" },
    ...healthKinds.map((k) => ({ value: k, label: kindLabels[k] })),
    ...(offerImported ? [{ value: importedFilter, label: importedLabel }] : []),
  ]
  return (
    <Select
      items={items}
      value={kind ?? allValue}
      onValueChange={(next) =>
        onKind(
          next === null || next === allValue ? null : (next as HealthFilter)
        )
      }
    >
      <SelectTrigger aria-label="Problem" className="w-56">
        <SelectValue />
      </SelectTrigger>
      <SelectContent alignItemWithTrigger={false} align="start">
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
      <SelectContent alignItemWithTrigger={false} align="start">
        {items.map((item) => (
          <SelectItem key={item.value} value={item.value}>
            {item.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

function HealthColumns() {
  return (
    <thead>
      <tr>
        <HeaderCell>Dashboard</HeaderCell>
        <HeaderCell>Problems</HeaderCell>
        <HeaderCell>Checked</HeaderCell>
        <HeaderCell>Last opened</HeaderCell>
        <HeaderCell className="text-right">
          <span className="sr-only">Actions</span>
        </HeaderCell>
      </tr>
    </thead>
  )
}

const tableWidth = 880

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
    <TableCard
      label="Dashboards with problems"
      stackOnNarrow
      minWidth={tableWidth}
    >
      <HealthColumns />
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

const skeletonClass = "motion-safe:animate-none"

function HealthSkeleton() {
  return (
    <TableCard
      label="Dashboards with problems"
      stackOnNarrow
      minWidth={tableWidth}
    >
      <HealthColumns />
      <tbody aria-busy="true">
        {Array.from({ length: skeletonRows }, (_, index) => (
          <tr key={index}>
            <Cell className="align-top">
              <Skeleton className={`h-4 w-44 ${skeletonClass}`} />
              <Skeleton className={`mt-1.5 h-3 w-24 ${skeletonClass}`} />
            </Cell>
            <Cell className="align-top">
              <Skeleton className={`h-4 w-56 ${skeletonClass}`} />
              <Skeleton className={`mt-1.5 h-3 w-40 ${skeletonClass}`} />
            </Cell>
            <Cell className="align-top">
              <Skeleton className={`h-4 w-16 ${skeletonClass}`} />
            </Cell>
            <Cell className="align-top">
              <Skeleton className={`h-4 w-16 ${skeletonClass}`} />
            </Cell>
            <Cell data-cell="action" className="align-top">
              <Skeleton className={`ml-auto h-7 w-16 ${skeletonClass}`} />
            </Cell>
          </tr>
        ))}
      </tbody>
    </TableCard>
  )
}
