import type { RefObject } from "react"
import { useQuery } from "@tanstack/react-query"

import { CopyButton } from "@/components/copy-button"
import { Button } from "@/components/ui/button"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
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
import { LogDetails, type LogSelection } from "@/panels/logs/logs-panel"
import { SidePanel } from "@/shell/side-panel"

import { ContextTab } from "./context-panel"
import { linePermalink } from "./line-fields"
import { contextBlocker, OpenTraceLink } from "./logs-results"
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

function Group({
  title,
  note,
  details,
  fields,
}: {
  title: string
  note?: string
  details: readonly LogDetail[]
  fields: FieldResolver
}) {
  if (details.length === 0) return null
  return (
    <section aria-label={title} className="flex flex-col gap-1.5">
      <h3 className="flex items-baseline gap-2 text-[13px] font-medium">
        {title}
        <span className="text-xs font-normal text-muted-foreground">
          {note === undefined ? details.length : `${note}, ${details.length}`}
        </span>
      </h3>
      <LogDetails
        details={details}
        fields={fields}
        zone={undefined}
        layout="drawer"
      />
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
  const { datasource } = useLogsWorkspace()
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
  const groups = documentGroups(entry.details)
  const shownTab: DetailTab = tab === "context" && searchFamily ? "table" : tab
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
          value={shownTab}
          onValueChange={(next) => {
            const chosen = tabs.find((candidate) => candidate === next)
            if (chosen !== undefined) onTabChange(chosen)
          }}
          className="min-h-0 flex-1"
        >
          <TabsList variant="line">
            <TabsTrigger value="table">Table</TabsTrigger>
            <TabsTrigger value="json">JSON</TabsTrigger>
            {searchFamily ? null : (
              <TabsTrigger value="context">Context</TabsTrigger>
            )}
          </TabsList>
          <TabsContent value="table" className="flex flex-col gap-3">
            <p className="font-mono text-xs [overflow-wrap:anywhere] whitespace-pre-wrap">
              {entry.message}
            </p>
            {searchFamily ? (
              <>
                <Group
                  title="Core fields"
                  details={groups.core}
                  fields={fields}
                />
                <Group
                  title="Other fields"
                  note={datasource.index ?? datasource.name}
                  details={groups.other}
                  fields={fields}
                />
              </>
            ) : (
              <LogDetails
                details={entry.details}
                fields={fields}
                zone={undefined}
                layout="drawer"
              />
            )}
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
