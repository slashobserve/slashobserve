import { useRef } from "react"
import { useInfiniteQuery } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"
import { PlusIcon } from "lucide-react"

import { AuthorMark } from "@/components/author-mark"
import { EmptyState } from "@/components/empty-state"
import { FilterToolbar } from "@/components/filter-toolbar"
import { LoadProblem } from "@/components/load-problem"
import { PageHeader } from "@/components/page-header"
import { RelativeTime } from "@/components/relative-time"
import { StatusLabel } from "@/components/status-label"
import { Cell, HeaderCell, TableCard } from "@/components/table-card"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { markOf } from "@/lib/author"
import { observationLink } from "@/lib/links"
import { loadProblemText } from "@/service-accounts/queries"

import { NewObservationPanel } from "./new-observation-panel"
import { observationsInfiniteQuery, type Observation } from "./queries"

export type ObservationStateFilter = "open" | "resolved" | "all"

type Props = {
  state: ObservationStateFilter
  creating: boolean
  onChange: (next: { state: ObservationStateFilter; creating: boolean }) => void
}

const stateItems: { value: ObservationStateFilter; label: string }[] = [
  { value: "open", label: "Open" },
  { value: "resolved", label: "Resolved" },
  { value: "all", label: "All" },
]

function isStateFilter(value: unknown): value is ObservationStateFilter {
  return stateItems.some((item) => item.value === value)
}

const tableWidth = 720
const skeletonRows = 3
const tableLabel = "Observations"

export function ObservationList({ state, creating, onChange }: Props) {
  const newButton = useRef<HTMLButtonElement>(null)
  const list = useInfiniteQuery(
    observationsInfiniteQuery(state === "all" ? {} : { state })
  )
  const items = list.data?.pages.flatMap((page) => page.observations) ?? []
  const filtered = state !== "open"

  function clearFilters() {
    onChange({ state: "open", creating })
  }

  const newObservation = (
    <Button ref={newButton} onClick={() => onChange({ state, creating: true })}>
      <PlusIcon aria-hidden="true" />
      New observation
    </Button>
  )

  return (
    <>
      <PageHeader title="Observations" actions={newObservation} />
      <FilterToolbar
        label="Observation filters"
        onClear={filtered ? clearFilters : undefined}
      >
        <Tabs
          value={state}
          onValueChange={(next) => {
            if (isStateFilter(next)) onChange({ state: next, creating })
          }}
        >
          <TabsList variant="line" aria-label="State">
            {stateItems.map((item) => (
              <TabsTrigger key={item.value} value={item.value}>
                {item.label}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>
      </FilterToolbar>
      {list.isError && list.data === undefined ? (
        <LoadProblem
          title="Could not load observations"
          detail={loadProblemText(list.error)}
          retrying={list.isFetching}
          onRetry={() => void list.refetch()}
        />
      ) : list.data === undefined ? (
        <ObservationSkeleton />
      ) : items.length === 0 ? (
        <div className="rounded-card border bg-background">
          {filtered ? (
            <EmptyState
              title="No observations match these filters."
              action={
                <Button variant="ghost" size="sm" onClick={clearFilters}>
                  Clear filters
                </Button>
              }
            />
          ) : (
            <EmptyState title="No observations yet" action={newObservation}>
              An observation is a page of evidence and notes that people and
              agents build together.
            </EmptyState>
          )}
        </div>
      ) : (
        <>
          <TableCard label={tableLabel} stackOnNarrow minWidth={tableWidth}>
            <ObservationColumns />
            <tbody>
              {items.map((observation) => (
                <ObservationRow
                  key={observation.id}
                  observation={observation}
                />
              ))}
            </tbody>
          </TableCard>
          {list.hasNextPage ? (
            <div className="mt-3 flex justify-center">
              <Button
                variant="outline"
                disabled={list.isFetchingNextPage}
                aria-busy={list.isFetchingNextPage}
                onClick={() => void list.fetchNextPage()}
              >
                Load more
              </Button>
            </div>
          ) : null}
        </>
      )}
      {creating ? (
        <NewObservationPanel
          trigger={newButton}
          onClose={() => onChange({ state, creating: false })}
        />
      ) : null}
    </>
  )
}

function ObservationColumns() {
  return (
    <thead>
      <tr>
        <HeaderCell>Title</HeaderCell>
        <HeaderCell>State</HeaderCell>
        <HeaderCell>Opened by</HeaderCell>
        <HeaderCell>Updated</HeaderCell>
      </tr>
    </thead>
  )
}

function ObservationRow({ observation }: { observation: Observation }) {
  return (
    <tr>
      <Cell className="max-w-[48ch] font-medium wrap-anywhere">
        <Link
          {...observationLink(observation.id)}
          className="underline-offset-4 hover:underline"
        >
          {observation.title}
        </Link>
      </Cell>
      <Cell>
        {observation.state === "open" ? (
          <StatusLabel tone="ok">Open</StatusLabel>
        ) : (
          <StatusLabel tone="none">Resolved</StatusLabel>
        )}
      </Cell>
      <Cell>
        <span className="inline-flex items-center gap-2">
          <AuthorMark mark={markOf(observation.created_by)} size={20} />
          {observation.created_by.label}
        </span>
      </Cell>
      <Cell className="whitespace-nowrap text-muted-foreground">
        <RelativeTime iso={observation.updated_at} />
      </Cell>
    </tr>
  )
}

function ObservationSkeleton() {
  return (
    <TableCard label={tableLabel} stackOnNarrow minWidth={tableWidth}>
      <ObservationColumns />
      <tbody aria-busy="true">
        {Array.from({ length: skeletonRows }, (_, index) => (
          <tr key={index}>
            <Cell>
              <Skeleton className="h-4 w-56 motion-safe:animate-none" />
            </Cell>
            <Cell>
              <Skeleton className="h-4 w-14 motion-safe:animate-none" />
            </Cell>
            <Cell>
              <Skeleton className="h-4 w-28 motion-safe:animate-none" />
            </Cell>
            <Cell>
              <Skeleton className="h-4 w-12 motion-safe:animate-none" />
            </Cell>
          </tr>
        ))}
      </tbody>
    </TableCard>
  )
}
