import { useEffect, useId, useMemo, useRef, useState } from "react"
import { useQuery } from "@tanstack/react-query"
import {
  ArrowUpDownIcon,
  MinusIcon,
  PenLineIcon,
  PlusIcon,
  type LucideIcon,
} from "lucide-react"

import { api } from "@/api/client"
import { ApiError, unwrap } from "@/api/errors"
import { dashboardKeys } from "@/api/keys"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { dashboardVersionQuery } from "@/dashboards/queries"
import { cn } from "@/lib/utils"

import {
  groupChanges,
  panelTitles,
  type DiffGroup,
  type GroupedChange,
  type TitleMaps,
} from "./diff-groups"
import {
  formatValue,
  limitBlocks,
  shouldLabelQueries,
  type ValueBlock,
} from "./diff-values"
import { useVersionHistory } from "./use-version-history"

type Props = {
  dashboardId: string
  latestVersion: number
  from: number | undefined
  to: number | undefined
  focusRequest: { from: number; to: number } | null
  onFocused: () => void
  onChange: (range: { from: number; to: number }) => void
}

const valueLimit = 2000

const opLabels: Record<
  GroupedChange["op"],
  { label: string; icon: LucideIcon }
> = {
  add: { label: "Added", icon: PlusIcon },
  remove: { label: "Removed", icon: MinusIcon },
  replace: { label: "Changed", icon: PenLineIcon },
  reorder: { label: "Reordered", icon: ArrowUpDownIcon },
}

function useVersionTitles(dashboardId: string, from: number, to: number) {
  const fromQuery = useQuery({
    ...dashboardVersionQuery(dashboardId, from),
    enabled: from > 0,
  })
  const toQuery = useQuery(dashboardVersionQuery(dashboardId, to))
  const fromSpec = fromQuery.data?.version.spec
  const toSpec = toQuery.data?.version.spec
  const titles = useMemo<TitleMaps>(
    () => ({ from: panelTitles(fromSpec), to: panelTitles(toSpec) }),
    [fromSpec, toSpec]
  )
  const pending = (from > 0 && fromQuery.isPending) || toQuery.isPending
  return { titles, pending }
}

function optionList(loaded: number[], selected: number, withEmpty: boolean) {
  const versions = new Set([...loaded, selected])
  if (withEmpty) versions.delete(0)
  const items = [...versions]
    .sort((a, b) => b - a)
    .map((value) => ({ value, label: `v${value}` }))
  if (withEmpty) items.push({ value: 0, label: "Empty dashboard" })
  return items
}

function VersionSelect({
  label,
  value,
  items,
  onChange,
}: {
  label: string
  value: number
  items: { value: number; label: string }[]
  onChange: (value: number) => void
}) {
  const id = useId()
  return (
    <div className="flex items-center gap-2">
      <Label htmlFor={id} className="text-muted-foreground max-sm:w-9">
        {label}
      </Label>
      <Select
        items={items}
        value={value}
        onValueChange={(next) => {
          if (next !== null) onChange(next)
        }}
      >
        <SelectTrigger id={id} className="min-w-40">
          <SelectValue />
        </SelectTrigger>
        <SelectContent
          alignItemWithTrigger={false}
          align="start"
          className="min-w-(--anchor-width)"
        >
          {items.map((item) => (
            <SelectItem
              key={item.value}
              value={item.value}
              className="pl-[11px]"
            >
              {item.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  )
}

function BlockView({ block }: { block: ValueBlock }) {
  const labelId = useId()
  return (
    <div
      role={block.label ? "group" : undefined}
      aria-labelledby={block.label ? labelId : undefined}
    >
      {block.label ? (
        <p id={labelId} className="px-2.5 pt-1.5 text-xs text-muted-foreground">
          {block.label}
        </p>
      ) : null}
      <pre
        tabIndex={0}
        className="max-h-64 overflow-auto px-2.5 py-1.5 font-mono text-xs break-words whitespace-pre-wrap outline-none focus-visible:ring-2 focus-visible:ring-ring"
      >
        {block.text === "" ? (
          <span className="font-sans text-muted-foreground">(empty)</span>
        ) : (
          block.text
        )}
      </pre>
      {block.options !== "" ? (
        <p className="px-2.5 pb-1.5 text-xs wrap-anywhere text-muted-foreground">
          {block.options}
        </p>
      ) : null}
    </div>
  )
}

function ValueBox({
  caption,
  tone,
  blocks,
}: {
  caption: "Before" | "After"
  tone: "before" | "after"
  blocks: ValueBlock[]
}) {
  const [expanded, setExpanded] = useState(false)
  const limited = limitBlocks(blocks, valueLimit)
  const cutAll = !expanded && limited.cut
  const shown = cutAll ? limited.blocks : blocks
  return (
    <div
      role="group"
      aria-label={caption}
      className={cn(
        "min-w-0 rounded-md border",
        tone === "before" ? "bg-destructive/10" : "bg-threshold-ok/10"
      )}
    >
      <p className="px-2.5 pt-1.5 text-xs font-medium">{caption}</p>
      {shown.map((block, index) => (
        <BlockView key={index} block={block} />
      ))}
      {cutAll ? (
        <Button
          variant="ghost"
          size="xs"
          className="mx-1.5 mb-1.5"
          onClick={() => setExpanded(true)}
        >
          Show all
        </Button>
      ) : null}
    </div>
  )
}

function ChangeRow({
  change,
  titles,
}: {
  change: GroupedChange
  titles: TitleMaps
}) {
  const { label, icon: Icon } = opLabels[change.op]
  const reorder = change.op === "reorder"
  const showBefore =
    change.op === "replace" || change.op === "remove" || reorder
  const showAfter = change.op === "replace" || change.op === "add" || reorder
  const labelQueries = shouldLabelQueries(
    change.field,
    reorder,
    change.from,
    change.to
  )
  return (
    <li className="flex flex-col gap-2 py-3 first:pt-0 last:pb-0">
      <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-sm">
        <span className="inline-flex items-center gap-1 font-medium">
          <Icon aria-hidden="true" className="size-3.5" />
          {label}
        </span>
        {change.field !== "" ? (
          <span className="wrap-anywhere text-muted-foreground">
            {change.field}
          </span>
        ) : null}
      </div>
      <div
        className={cn(
          "grid gap-2",
          showBefore && showAfter && "grid-cols-1 md:grid-cols-2"
        )}
      >
        {showBefore ? (
          <ValueBox
            caption="Before"
            tone="before"
            blocks={formatValue(
              change.field,
              change.from,
              titles,
              reorder,
              labelQueries
            )}
          />
        ) : null}
        {showAfter ? (
          <ValueBox
            caption="After"
            tone="after"
            blocks={formatValue(
              change.field,
              change.to,
              titles,
              reorder,
              labelQueries
            )}
          />
        ) : null}
      </div>
    </li>
  )
}

function GroupSection({
  group,
  titles,
}: {
  group: DiffGroup
  titles: TitleMaps
}) {
  const headingId = useId()
  return (
    <section
      aria-labelledby={headingId}
      className="rounded-lg border bg-card p-4 text-card-foreground"
    >
      <h3 id={headingId} className="mb-3 text-sm font-semibold wrap-anywhere">
        {group.label}
      </h3>
      <ul className="divide-y">
        {group.changes.map((change, index) => (
          <ChangeRow key={index} change={change} titles={titles} />
        ))}
      </ul>
    </section>
  )
}

function DiffSkeleton() {
  return (
    <div className="flex flex-col gap-3" aria-busy="true">
      <Skeleton className="h-24 w-full" />
      <Skeleton className="h-24 w-full" />
    </div>
  )
}

function DiffBody({
  dashboardId,
  from,
  to,
}: {
  dashboardId: string
  from: number
  to: number
}) {
  const diff = useQuery({
    queryKey: dashboardKeys.diff(dashboardId, from, to),
    queryFn: ({ signal }) =>
      api
        .GET("/dashboards/{id}/diff", {
          params: { path: { id: dashboardId }, query: { from, to } },
          signal,
        })
        .then(unwrap),
    staleTime: Infinity,
  })
  const { titles, pending } = useVersionTitles(dashboardId, from, to)
  const groups = useMemo(
    () => (diff.data ? groupChanges(diff.data.changes, titles) : []),
    [diff.data, titles]
  )

  if (diff.isError) {
    if (diff.error instanceof ApiError && diff.error.status === 404) {
      return (
        <p role="status" className="text-sm text-muted-foreground">
          One of these versions does not exist.
        </p>
      )
    }
    return (
      <Alert variant="destructive">
        <AlertTitle>Could not load the changes</AlertTitle>
        <AlertDescription>
          <p>{diff.error.message}</p>
          <Button
            variant="outline"
            size="sm"
            className="text-foreground"
            disabled={diff.isFetching}
            focusableWhenDisabled
            aria-busy={diff.isFetching}
            onClick={() => void diff.refetch()}
          >
            Retry
          </Button>
        </AlertDescription>
      </Alert>
    )
  }
  if (diff.isPending || pending) return <DiffSkeleton />
  if (diff.data.changes.length === 0) {
    return <p className="text-sm text-muted-foreground">No differences</p>
  }
  return (
    <div className="flex flex-col gap-3">
      {groups.map((group) => (
        <GroupSection key={group.key} group={group} titles={titles} />
      ))}
    </div>
  )
}

export function VersionDiff({
  dashboardId,
  latestVersion,
  from: fromSearch,
  to: toSearch,
  focusRequest,
  onFocused,
  onChange,
}: Props) {
  const heading = useRef<HTMLHeadingElement>(null)
  const history = useVersionHistory(dashboardId)
  const to = toSearch ?? latestVersion
  const from = fromSearch ?? Math.max(to - 1, 0)

  useEffect(() => {
    if (focusRequest === null) return
    if (focusRequest.from !== from || focusRequest.to !== to) return
    heading.current?.focus()
    heading.current?.scrollIntoView({ block: "start" })
    onFocused()
  }, [focusRequest, from, to, onFocused])

  const loaded = useMemo(
    () =>
      history.data?.pages.flatMap((page) =>
        page.versions.map((meta) => meta.version)
      ) ?? [],
    [history.data]
  )
  const fromItems = useMemo(
    () => optionList(loaded, from, true),
    [loaded, from]
  )
  const toItems = useMemo(() => optionList(loaded, to, false), [loaded, to])

  return (
    <section aria-labelledby="diff-heading" className="mt-8">
      <h2
        id="diff-heading"
        ref={heading}
        tabIndex={-1}
        className="mb-3 text-lg font-semibold outline-none"
      >
        Changes from {from === 0 ? "an empty dashboard" : `v${from}`} to v{to}
      </h2>
      <div className="mb-4 flex flex-wrap items-center gap-x-4 gap-y-2 max-sm:flex-col max-sm:items-start">
        <VersionSelect
          label="From"
          value={from}
          items={fromItems}
          onChange={(next) => onChange({ from: next, to })}
        />
        <VersionSelect
          label="To"
          value={to}
          items={toItems}
          onChange={(next) => onChange({ from, to: next })}
        />
      </div>
      {from >= to ? (
        <p role="status" className="text-sm text-muted-foreground">
          Pick an older version on the left.
        </p>
      ) : (
        <DiffBody
          key={`${from}:${to}`}
          dashboardId={dashboardId}
          from={from}
          to={to}
        />
      )}
    </section>
  )
}
