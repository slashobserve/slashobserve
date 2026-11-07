import type { ReactNode, RefObject } from "react"
import { useQuery } from "@tanstack/react-query"
import { AsteriskIcon, FilterIcon, FilterXIcon } from "lucide-react"

import { CopyButton } from "@/components/copy-button"
import { Button } from "@/components/ui/button"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import { useDatasources } from "@/datasources/queries"
import { zonedDateTimeMs } from "@/lib/time"
import { cn } from "@/lib/utils"
import { labelNamesQuery } from "@/panel-editor/queries"
import type { FieldResolver } from "@/panels/field-config"
import {
  detailText,
  type LogDetail,
  type LogEntry,
} from "@/panels/logs/logs-model"
import type { LogSelection } from "@/panels/logs/logs-panel"
import { SidePanel } from "@/shell/side-panel"

import { ContextTab } from "./context-panel"
import { linePermalink } from "./line-fields"
import { contextBlocker, OpenTraceLink } from "./logs-results"
import {
  filterTarget,
  isStreamLabel,
  planFilter,
  type FilterAction,
  type FilterPlan,
} from "./query-filters"
import { isSearchFamily } from "./search"
import { traceDatasourceFor, traceIdOf } from "./trace-link"
import { useLogsWorkspace, type DetailTab } from "./workspace"

const coreNames = ["time", "level", "service", "trace_id", "span_id", "id"]
const hiddenNames = new Set(["body", "timens"])
const expandedPrefix = "Fields."
const tracePrefixLength = 8
const tabs: readonly DetailTab[] = ["table", "json", "context"]

export function documentGroups(details: readonly LogDetail[]): {
  core: LogDetail[]
  other: LogDetail[]
} {
  const core: LogDetail[] = []
  const other: LogDetail[] = []
  for (const detail of details) {
    const lower = detail.name.toLowerCase()
    if (hiddenNames.has(lower)) continue
    if (coreNames.includes(lower)) {
      core.push(detail)
    } else if (lower === "index") {
      other.push({ ...detail, name: "_index" })
    } else if (detail.name.startsWith(expandedPrefix)) {
      other.push({ ...detail, name: detail.name.slice(expandedPrefix.length) })
    } else {
      other.push(detail)
    }
  }
  core.sort(
    (a, b) =>
      coreNames.indexOf(a.name.toLowerCase()) -
      coreNames.indexOf(b.name.toLowerCase())
  )
  return { core, other }
}

export function entryJson(
  entry: LogEntry,
  fields: FieldResolver
): Record<string, unknown> {
  return {
    time: entry.time === null ? null : new Date(entry.time).toISOString(),
    level: entry.level?.word ?? null,
    message: entry.message,
    labels: entry.labels,
    fields: Object.fromEntries(
      entry.details.map((detail) => [
        detail.name,
        detailText(fields, detail, undefined),
      ])
    ),
  }
}

function indexName({ entry, fields }: LogSelection): string | undefined {
  const detail = entry.details.find(
    (candidate) => candidate.name.toLowerCase() === "index"
  )
  const text = detail ? detailText(fields, detail, undefined) : ""
  return text === "" ? undefined : text
}

type FieldRowData = { name: string; value: string; filterable: boolean }

type FieldGroup = { title: string; note?: string; rows: FieldRowData[] }

function shownDetail(detail: LogDetail, text: string, message: string) {
  return !(
    ("field" in detail && detail.field.type === "time") ||
    hiddenNames.has(detail.name.toLowerCase()) ||
    detail.name === "__name__" ||
    text === message
  )
}

function lokiGroups(
  selection: LogSelection,
  indexed: readonly string[] | undefined
): FieldGroup[] {
  const { entry, fields, streamLabels } = selection
  const labels: FieldRowData[] = []
  const metadata: FieldRowData[] = []
  for (const name of Object.keys(streamLabels).sort()) {
    const row = { name, value: streamLabels[name], filterable: true }
    const indexedLabel = isStreamLabel(name, streamLabels, indexed)
    ;(indexedLabel ? labels : metadata).push(row)
  }
  const parsed = entry.details.flatMap((detail): FieldRowData[] => {
    const value = detailText(fields, detail, undefined)
    return detail.name in streamLabels ||
      !shownDetail(detail, value, entry.message)
      ? []
      : [{ name: detail.name, value, filterable: true }]
  })
  return [
    { title: "Labels", note: "Indexed by Loki", rows: labels },
    { title: "Parsed fields", note: "From the json stage", rows: parsed },
    {
      title: "Structured metadata",
      note: "Stored with the line, not indexed",
      rows: metadata,
    },
  ]
}

function clickhouseGroups(selection: LogSelection): FieldGroup[] {
  const { entry, fields } = selection
  const columns: FieldRowData[] = []
  const attributes: FieldRowData[] = []
  for (const detail of entry.details) {
    const value = detailText(fields, detail, undefined)
    if (!shownDetail(detail, value, entry.message)) continue
    const row = { name: detail.name, value, filterable: true }
    const column = filterTarget("clickhouse", detail.name, false)
    ;(column === "clickhouse-column" ? columns : attributes).push(row)
  }
  return [
    { title: "Columns", rows: columns },
    { title: "Attributes", note: "From LogAttributes", rows: attributes },
  ]
}

function searchGroups(selection: LogSelection, index: string): FieldGroup[] {
  const { core, other } = documentGroups(selection.entry.details)
  const rowOf = (detail: LogDetail, filterable: boolean): FieldRowData => ({
    name: detail.name,
    value: detailText(selection.fields, detail, undefined),
    filterable,
  })
  return [
    { title: "Core fields", rows: core.map((detail) => rowOf(detail, false)) },
    {
      title: "Other fields",
      note: index,
      rows: other.map((detail) => rowOf(detail, detail.name !== "_index")),
    },
  ]
}

function fieldGroups(
  type: string,
  selection: LogSelection,
  indexed: readonly string[] | undefined,
  index: string
): FieldGroup[] {
  const groups = isSearchFamily(type)
    ? searchGroups(selection, index)
    : type === "clickhouse"
      ? clickhouseGroups(selection)
      : lokiGroups(selection, indexed)
  return groups.filter((group) => group.rows.length > 0)
}

const ariaValueLength = 60

function RowAction({
  label,
  tooltip,
  plan,
  onClick,
  children,
}: {
  label: string
  tooltip: string
  plan: FilterPlan
  onClick: () => void
  children: ReactNode
}) {
  const blocked = "blocked" in plan ? plan.blocked : undefined
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <Button
            type="button"
            variant="ghost"
            size="icon-xs"
            className="size-5"
            aria-label={label}
            focusableWhenDisabled
            disabled={blocked !== undefined}
            onClick={onClick}
          />
        }
      >
        {children}
      </TooltipTrigger>
      <TooltipContent>{blocked ?? tooltip}</TooltipContent>
    </Tooltip>
  )
}

function FieldRow({
  row,
  searchFamily,
  planFor,
  onFilter,
}: {
  row: FieldRowData
  searchFamily: boolean
  planFor: (action: FilterAction, field: string, value: string) => FilterPlan
  onFilter: (action: FilterAction, field: string, value: string) => void
}) {
  const { name, value } = row
  const shown =
    value.length > ariaValueLength
      ? `${value.slice(0, ariaValueLength)}...`
      : value
  const action = (
    kind: FilterAction,
    label: string,
    tooltip: string,
    icon: ReactNode
  ) => (
    <RowAction
      label={label}
      tooltip={tooltip}
      plan={planFor(kind, name, value)}
      onClick={() => onFilter(kind, name, value)}
    >
      {icon}
    </RowAction>
  )
  return (
    <div className="group grid min-h-7 grid-cols-[minmax(0,8rem)_minmax(0,1fr)_auto] items-start gap-x-3">
      <dt className="font-mono [overflow-wrap:anywhere] text-muted-foreground">
        {name}
      </dt>
      <dd className="font-mono [overflow-wrap:anywhere] whitespace-pre-wrap">
        {value}
      </dd>
      <dd className="flex gap-0.5 opacity-0 group-focus-within:opacity-100 group-hover:opacity-100">
        {row.filterable ? (
          <>
            {action(
              "in",
              `Include ${name} ${shown}`,
              "Filter in",
              <FilterIcon aria-hidden />
            )}
            {action(
              "out",
              `Exclude ${name} ${shown}`,
              "Filter out",
              <FilterXIcon aria-hidden />
            )}
            {searchFamily
              ? action(
                  "exists",
                  `Require ${name}`,
                  "Field exists",
                  <AsteriskIcon aria-hidden />
                )
              : null}
          </>
        ) : null}
      </dd>
    </div>
  )
}

function Group({
  group,
  searchFamily,
  planFor,
  onFilter,
}: {
  group: FieldGroup
  searchFamily: boolean
  planFor: (action: FilterAction, field: string, value: string) => FilterPlan
  onFilter: (action: FilterAction, field: string, value: string) => void
}) {
  const { title, note, rows } = group
  return (
    <section aria-label={title} className="flex flex-col gap-1.5">
      <h3 className="flex items-baseline gap-2 text-[13px] font-medium">
        {title}
        <span className="text-xs font-normal text-muted-foreground">
          {note === undefined ? rows.length : `${note}, ${rows.length}`}
        </span>
      </h3>
      <dl className="flex flex-col text-xs">
        {rows.map((row, index) => (
          <FieldRow
            key={`${index}:${row.name}`}
            row={row}
            searchFamily={searchFamily}
            planFor={planFor}
            onFilter={onFilter}
          />
        ))}
      </dl>
    </section>
  )
}

export function DetailDrawer({
  selection,
  tab,
  onTabChange,
  onClose,
  trigger,
  id,
}: {
  selection: LogSelection
  tab: DetailTab
  onTabChange: (tab: DetailTab) => void
  onClose: () => void
  trigger: RefObject<HTMLElement | null>
  id: string
}) {
  const { datasource, search, run } = useLogsWorkspace()
  const allDatasources = useDatasources()
  const indexLabels = useQuery({
    ...labelNamesQuery(datasource.uid, {}),
    enabled: datasource.type === "loki",
  })
  const { entry, fields } = selection
  const searchFamily = isSearchFamily(datasource.type)
  const traceDatasource = traceDatasourceFor(
    datasource,
    allDatasources.data ?? []
  )
  const traceId = traceIdOf(entry)
  const blocker = contextBlocker(datasource, entry)
  const groups = fieldGroups(
    datasource.type,
    selection,
    indexLabels.data?.labels,
    datasource.index ?? datasource.name
  )

  function planLineFilter(
    action: FilterAction,
    field: string,
    value: string
  ): FilterPlan {
    return planFilter(
      {
        type: datasource.type,
        query: search.q ?? "",
        stream: isStreamLabel(
          field,
          selection.streamLabels,
          indexLabels.data?.labels
        ),
      },
      action,
      field,
      value
    )
  }

  function filterFromLine(action: FilterAction, field: string, value: string) {
    const plan = planLineFilter(action, field, value)
    if ("text" in plan) run(plan.text)
  }

  const description = [
    searchFamily ? indexName(selection) : undefined,
    entry.time === null ? undefined : zonedDateTimeMs(entry.time, undefined),
    entry.level?.word,
  ]
    .filter((part) => part !== undefined)
    .join(", ")

  return (
    <SidePanel
      title={searchFamily ? "Document" : "Line"}
      description={description === "" ? undefined : description}
      onClose={onClose}
      trigger={trigger}
    >
      <div id={id} className="flex min-h-0 flex-1 flex-col gap-3">
        <div
          role="group"
          aria-label="Line actions"
          className="flex flex-wrap gap-2"
        >
          <Button
            type="button"
            variant="outline"
            size="xs"
            disabled={blocker !== undefined}
            title={blocker}
            onClick={() => onTabChange("context")}
          >
            {searchFamily ? "Surrounding documents" : "Show context"}
          </Button>
          <CopyButton
            text={entry.message}
            label="Copy line"
            variant="outline"
            size="xs"
            icon={false}
          >
            Copy line
          </CopyButton>
          <CopyButton
            text={linePermalink(window.location.href, entry)}
            label="Copy link"
            variant="outline"
            size="xs"
            icon={false}
          >
            Copy link
          </CopyButton>
        </div>
        {traceId !== null && traceDatasource !== null && entry.time !== null ? (
          <div className="rounded-card border border-border px-3 py-2 text-xs">
            <OpenTraceLink
              traceId={traceId}
              traceDatasource={traceDatasource}
              time={entry.time}
              className={cn("underline underline-offset-3")}
            >
              Open trace {traceId.slice(0, tracePrefixLength)}
            </OpenTraceLink>
          </div>
        ) : null}
        <Tabs
          value={tab}
          onValueChange={(next) => {
            const chosen = tabs.find((candidate) => candidate === next)
            if (chosen !== undefined) onTabChange(chosen)
          }}
          className="min-h-0 flex-1"
        >
          <TabsList variant="line">
            <TabsTrigger value="table">Table</TabsTrigger>
            <TabsTrigger value="json">JSON</TabsTrigger>
            <TabsTrigger value="context">Context</TabsTrigger>
          </TabsList>
          <TabsContent value="table" className="flex flex-col gap-3">
            <p className="font-mono text-xs [overflow-wrap:anywhere] whitespace-pre-wrap">
              {entry.message}
            </p>
            {groups.map((group) => (
              <Group
                key={group.title}
                group={group}
                searchFamily={searchFamily}
                planFor={planLineFilter}
                onFilter={filterFromLine}
              />
            ))}
          </TabsContent>
          <TabsContent value="json">
            <pre className="font-mono text-xs [overflow-wrap:anywhere] whitespace-pre-wrap">
              {JSON.stringify(entryJson(entry, fields), null, 2)}
            </pre>
          </TabsContent>
          <TabsContent value="context" className="flex min-h-0 flex-col gap-2">
            <ContextTab
              datasource={datasource}
              anchor={selection}
              indexedLabels={indexLabels.data?.labels}
            />
          </TabsContent>
        </Tabs>
      </div>
    </SidePanel>
  )
}
