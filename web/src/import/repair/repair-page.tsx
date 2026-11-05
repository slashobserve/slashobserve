import { useId, useState } from "react"
import { useInfiniteQuery, useQuery } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"
import { LoaderCircleIcon } from "lucide-react"
import { toast } from "sonner"

import { LoadProblem } from "@/components/load-problem"
import { PageHeader } from "@/components/page-header"
import { RelativeTime } from "@/components/relative-time"
import { StatusLabel } from "@/components/status-label"
import { Cell, HeaderCell, TableCard } from "@/components/table-card"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
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
import { useAgentInfo } from "@/observe/queries"
import { loadProblemText } from "@/service-accounts/queries"

import { repairsQuery, useRequestRepairs, type RepairsQueued } from "./queries"
import {
  maxActiveRepairs,
  outcomeLines,
  repairState,
  unverifiedText,
  type RepairItem,
} from "./repair-state"

type Props = {
  folderId: string | null
  onFolder: (folderId: string | null) => void
}

const allFolders = "all"

function announce(outcome: RepairsQueued) {
  const [first, ...rest] = outcomeLines(outcome)
  toast.success(
    first,
    rest.length === 0 ? undefined : { description: rest.join(" ") }
  )
}

export function RepairPage({ folderId, onFolder }: Props) {
  const agent = useAgentInfo()
  const agentAvailable = agent.data?.available ?? false
  const list = useInfiniteQuery(repairsQuery(folderId))
  const request = useRequestRepairs()
  const items = list.data?.pages.flatMap((page) => page.dashboards) ?? []
  const pendingTarget = request.isPending ? request.variables : undefined
  const pendingId =
    pendingTarget !== undefined && "dashboardId" in pendingTarget
      ? pendingTarget.dashboardId
      : null
  const bulkPending = pendingTarget !== undefined && pendingId === null

  function requestRepairs(target: Parameters<typeof request.mutate>[0]) {
    request.mutate(target, {
      onSuccess: announce,
      onError: (error) => toast.error(loadProblemText(error)),
    })
  }

  return (
    <>
      <PageHeader
        title="Repair queue"
        description="Dashboards whose panels or variables failed their queries, or hold placeholders. The agent repairs failing queries as a revision; a dashboard leaves this list once nothing in it fails."
        actions={
          <>
            <FolderSelect folderId={folderId} onFolder={onFolder} />
            <RepairAll
              disabled={!agentAvailable || items.length === 0}
              pending={bulkPending}
              onConfirm={() => requestRepairs({ all: true, folderId })}
            />
          </>
        }
      />
      {agent.data !== undefined && !agentAvailable ? (
        <p className="mb-4 text-[13px] text-muted-foreground">
          <StatusLabel tone="warning">
            No agent is configured. Set SLASHOBSERVE_LLM_PROVIDER and restart.
          </StatusLabel>
        </p>
      ) : null}
      {list.isError && list.data === undefined ? (
        <LoadProblem
          title="Could not load the repair queue"
          detail={loadProblemText(list.error)}
          retrying={list.isFetching}
          onRetry={() => void list.refetch()}
        />
      ) : list.data === undefined ? (
        <Skeleton className="h-32 w-full" />
      ) : items.length === 0 ? (
        <p className="rounded-card border bg-background px-4 py-6 text-[13px] text-muted-foreground">
          No imported dashboard has failing panels or placeholders.
        </p>
      ) : (
        <>
          <RepairTable
            items={items}
            agentAvailable={agentAvailable}
            pendingId={pendingId}
            onRepair={(dashboardId) => requestRepairs({ dashboardId })}
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

function FolderSelect({ folderId, onFolder }: Props) {
  const folders = useQuery(foldersQuery())
  const items = [
    { value: allFolders, label: "All folders" },
    ...(folders.data ?? []).map((folder) => ({
      value: folder.id,
      label: folder.title,
    })),
  ]
  return (
    <Select
      items={items}
      value={folderId ?? allFolders}
      onValueChange={(next) =>
        onFolder(next === null || next === allFolders ? null : next)
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

function RepairAll({
  disabled,
  pending,
  onConfirm,
}: {
  disabled: boolean
  pending: boolean
  onConfirm: () => void
}) {
  const [open, setOpen] = useState(false)
  return (
    <>
      <Button
        disabled={disabled || pending}
        focusableWhenDisabled
        aria-busy={pending}
        onClick={() => setOpen(true)}
      >
        {pending ? (
          <LoaderCircleIcon
            aria-hidden="true"
            className="motion-safe:animate-spin"
          />
        ) : null}
        Repair all
      </Button>
      <AlertDialog open={open} onOpenChange={setOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              Queue a repair for every dashboard in this list?
            </AlertDialogTitle>
            <AlertDialogDescription>
              Each one is an agent run, queued behind other requests. You can
              have {maxActiveRepairs} repairs queued or running at a time.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={() => {
                setOpen(false)
                onConfirm()
              }}
            >
              Repair all
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}

function RepairTable({
  items,
  agentAvailable,
  pendingId,
  onRepair,
}: {
  items: RepairItem[]
  agentAvailable: boolean
  pendingId: string | null
  onRepair: (dashboardId: string) => void
}) {
  return (
    <TableCard label="Dashboards to repair" minWidth={960}>
      <thead>
        <tr>
          <HeaderCell>Dashboard</HeaderCell>
          <HeaderCell>Folder</HeaderCell>
          <HeaderCell>Unverified</HeaderCell>
          <HeaderCell>Placeholders</HeaderCell>
          <HeaderCell>Imported</HeaderCell>
          <HeaderCell>Repair</HeaderCell>
          <HeaderCell>
            <span className="sr-only">Actions</span>
          </HeaderCell>
        </tr>
      </thead>
      <tbody>
        {items.map((item) => (
          <RepairRow
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

function RepairRow({
  item,
  agentAvailable,
  pending,
  onRepair,
}: {
  item: RepairItem
  agentAvailable: boolean
  pending: boolean
  onRepair: () => void
}) {
  const reasonId = useId()
  const state = repairState(item, agentAvailable)
  return (
    <tr>
      <Cell className="wrap-anywhere">
        <Link
          {...dashboardLink(item.id, item.title)}
          className="font-medium underline-offset-2 hover:underline"
        >
          {item.title}
        </Link>
      </Cell>
      <Cell className="wrap-anywhere">{item.folder.title}</Cell>
      <Cell>{unverifiedText(item)}</Cell>
      <Cell>{item.placeholder_panels}</Cell>
      <Cell className="whitespace-nowrap">
        <RelativeTime iso={item.last_import_at} fallback="Not imported" />
      </Cell>
      <Cell>
        {state.tone === null || state.label === null ? (
          <span className="text-muted-foreground">Not requested</span>
        ) : (
          <div className="flex flex-col gap-0.5">
            <StatusLabel tone={state.tone}>{state.label}</StatusLabel>
            {item.repair?.finished_at ? (
              <span className="text-xs text-muted-foreground">
                <RelativeTime iso={item.repair.finished_at} />
              </span>
            ) : null}
          </div>
        )}
      </Cell>
      <Cell className="text-right">
        <Button
          variant="outline"
          size="sm"
          disabled={!state.canRepair || pending}
          focusableWhenDisabled
          aria-busy={pending}
          aria-describedby={state.reason === null ? undefined : reasonId}
          title={state.reason ?? undefined}
          onClick={onRepair}
        >
          {pending ? (
            <LoaderCircleIcon
              aria-hidden="true"
              className="motion-safe:animate-spin"
            />
          ) : null}
          Repair with the agent
        </Button>
        {state.reason === null ? null : (
          <span id={reasonId} className="sr-only">
            {state.reason}
          </span>
        )}
      </Cell>
    </tr>
  )
}
