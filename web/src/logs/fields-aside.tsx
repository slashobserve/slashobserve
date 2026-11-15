import { useId, useMemo, useState } from "react"
import { useQuery } from "@tanstack/react-query"
import { ChevronRightIcon, FilterXIcon, PanelLeftCloseIcon } from "lucide-react"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import { cn } from "@/lib/utils"
import { labelNamesQuery } from "@/panel-editor/queries"

import {
  barPercent,
  basisNote,
  facetBasis,
  facetCountText,
  facetsErrorText,
  fieldRows,
  fieldsNote,
  filterQuery,
  kindBadge,
  logFacetsQuery,
  splitFields,
  storedFacetsOpen,
  storeFacetsOpen,
  topValues,
  type FacetField,
  type FacetValue,
  type FieldRow,
  type FilterMode,
  type LogFacets,
} from "./facets"
import { useLogsWorkspace } from "./workspace"

const asideClass = "max-[1000px]:hidden"
const iconButtonClass = "size-5 text-muted-foreground"

type Filter = (field: string, value: string, mode: FilterMode) => string | null

type Base = { data: LogFacets | undefined; pending: boolean }

function ValueRow({
  field,
  value,
  filter,
  apply,
}: {
  field: FacetField
  value: FacetValue
  filter: Filter
  apply: (query: string) => void
}) {
  const count = facetCountText(field, value)
  const unit = field.unit === "bytes" ? "" : " lines"
  const included = filter(field.name, value.value, "in")
  const excluded = filter(field.name, value.value, "out")
  return (
    <div
      role="group"
      aria-label={`${value.value}, ${count}${unit}`}
      className="grid grid-cols-[minmax(0,1fr)_2.5rem_auto] items-center gap-1.5 rounded-control py-0.5 pl-1 hover:bg-muted"
    >
      <div className="flex min-w-0 flex-col gap-0.5">
        <button
          type="button"
          title={value.value}
          aria-label={`Include ${field.name} ${value.value}`}
          disabled={included === null}
          className="truncate text-left font-mono text-xs outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed"
          onClick={() => included !== null && apply(included)}
        >
          {value.value === "" ? "(empty)" : value.value}
        </button>
        <span
          aria-hidden="true"
          className="h-1 rounded-control bg-series-1/60"
          style={{ width: `${barPercent(field, value)}%` }}
        />
      </div>
      <span className="text-right text-xs text-muted-foreground tabular-nums">
        {count}
      </span>
      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              type="button"
              variant="ghost"
              size="icon-xs"
              className={iconButtonClass}
              aria-label={`Exclude ${field.name} ${value.value}`}
              disabled={excluded === null}
              onClick={() => excluded !== null && apply(excluded)}
            />
          }
        >
          <FilterXIcon aria-hidden="true" />
        </TooltipTrigger>
        <TooltipContent>
          {excluded === null ? "Cannot edit this query" : "Filter out"}
        </TooltipContent>
      </Tooltip>
    </div>
  )
}

function FieldValues({
  row,
  base,
  filter,
  apply,
}: {
  row: FieldRow
  base: Base
  filter: Filter
  apply: (query: string) => void
}) {
  const { datasource, search, effectiveQuery, range } = useLogsWorkspace()
  const q = search.q
  const own = useQuery({
    ...logFacetsQuery(datasource.uid, {
      query: effectiveQuery ?? "",
      from: range.from,
      to: range.to,
      limit: topValues,
      fields: [row.name],
    }),
    enabled: q !== undefined && row.facet === null && !base.pending,
  })
  if (q === undefined) {
    return <p className="m-0 text-muted-foreground">Run a query first.</p>
  }
  const source = row.facet === null ? own.data : base.data
  const field =
    row.facet ?? own.data?.fields.find((item) => item.name === row.name)
  if (base.pending || (row.facet === null && own.isPending)) {
    return <Skeleton aria-hidden="true" className="h-8" />
  }
  if (
    source === undefined ||
    field === undefined ||
    field.values.length === 0
  ) {
    return <p className="m-0 text-muted-foreground">No values in this range.</p>
  }
  return (
    <>
      <div className="flex items-center justify-between gap-2">
        <span className="font-medium">Top {field.values.length} values</span>
        <span className="text-muted-foreground">
          {facetBasis(source, field)}
        </span>
      </div>
      <div className="flex flex-col">
        {field.values.map((value) => (
          <ValueRow
            key={value.value}
            field={field}
            value={value}
            filter={filter}
            apply={apply}
          />
        ))}
      </div>
      <p className="m-0 text-muted-foreground">{basisNote(source, field)}</p>
    </>
  )
}

function FieldItem({
  row,
  expanded,
  onToggle,
  base,
  filter,
  apply,
}: {
  row: FieldRow
  expanded: boolean
  onToggle: () => void
  base: Base
  filter: Filter
  apply: (query: string) => void
}) {
  const panelId = useId()
  return (
    <li>
      <button
        type="button"
        aria-expanded={expanded}
        aria-controls={panelId}
        className="flex h-7 w-full items-center gap-1.5 rounded-control pr-1 pl-1.5 text-left text-xs outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring aria-expanded:bg-selection"
        onClick={onToggle}
      >
        <ChevronRightIcon
          aria-hidden="true"
          className={cn(
            "size-3 shrink-0 text-muted-foreground",
            expanded && "rotate-90"
          )}
        />
        <span
          aria-label={row.facet?.kind}
          className="inline-grid h-4 w-[18px] shrink-0 place-items-center rounded-control bg-muted font-mono text-[9px] font-semibold text-muted-foreground"
        >
          {row.facet ? kindBadge(row.facet.kind) : null}
        </span>
        <span className="min-w-0 flex-1 truncate font-mono" title={row.name}>
          {row.name}
        </span>
      </button>
      {expanded ? (
        <div
          id={panelId}
          role="group"
          aria-label={`Top values of ${row.name}`}
          className="flex flex-col gap-1.5 py-1.5 pr-1 pl-5 text-xs"
        >
          <FieldValues row={row} base={base} filter={filter} apply={apply} />
        </div>
      ) : null}
    </li>
  )
}

function FieldGroup({
  title,
  rows,
  openField,
  setOpenField,
  base,
  filter,
  apply,
}: {
  title: string
  rows: readonly FieldRow[]
  openField: string | null
  setOpenField: (name: string | null) => void
  base: Base
  filter: Filter
  apply: (query: string) => void
}) {
  if (rows.length === 0) return null
  return (
    <section
      aria-label={`${title} fields`}
      className="flex flex-col gap-0.5 py-1"
    >
      <h3 className="m-0 px-1.5 py-1 text-xs font-medium text-muted-foreground">
        {title}
        <span className="ml-1.5 font-normal tabular-nums">{rows.length}</span>
      </h3>
      <ul className="m-0 flex list-none flex-col gap-px p-0">
        {rows.map((row) => (
          <FieldItem
            key={row.name}
            row={row}
            expanded={openField === row.name}
            onToggle={() =>
              setOpenField(openField === row.name ? null : row.name)
            }
            base={base}
            filter={filter}
            apply={apply}
          />
        ))}
      </ul>
    </section>
  )
}

export function FieldsAside() {
  const { datasource, search, effectiveQuery, range, setDraft, run, detail } =
    useLogsWorkspace()
  const [shown, setShown] = useState(storedFacetsOpen)
  const lineOpen = detail.selection !== null
  const [lineWasOpen, setLineWasOpen] = useState(lineOpen)
  const [foldedByLine, setFoldedByLine] = useState(lineOpen)
  if (lineOpen !== lineWasOpen) {
    setLineWasOpen(lineOpen)
    setFoldedByLine(lineOpen)
  }
  const open = shown && !foldedByLine
  const [filterText, setFilterText] = useState("")
  const [openField, setOpenField] = useState<string | null>(null)
  const uid = datasource.uid
  const type = datasource.type
  const q = search.q
  const names = useQuery({
    ...labelNamesQuery(uid, { from: range.from, to: range.to }),
    enabled: open && type !== "clickhouse",
  })
  const facets = useQuery({
    ...logFacetsQuery(uid, {
      query: effectiveQuery ?? "",
      from: range.from,
      to: range.to,
      limit: topValues,
    }),
    enabled: open && q !== undefined,
  })
  const base: Base = {
    data: facets.data,
    pending: q !== undefined && facets.isPending,
  }
  const rows = useMemo(
    () => fieldRows(names.data?.labels ?? [], facets.data?.fields ?? []),
    [names.data, facets.data]
  )
  const selected = useMemo(
    () => (search.cols === undefined ? [] : search.cols.split(",")),
    [search.cols]
  )
  const groups = splitFields(rows, selected, filterText)

  function toggle(next: boolean) {
    setShown(next)
    setFoldedByLine(false)
    storeFacetsOpen(next)
  }

  const filter: Filter = (field, value, mode) =>
    q === undefined ? null : filterQuery(type, q, field, value, mode)

  function apply(query: string) {
    setDraft(query)
    run(query)
  }

  if (!open) {
    return (
      <aside
        aria-label="Fields, hidden"
        className={cn("w-9 shrink-0 border-r border-border", asideClass)}
      >
        <Button
          type="button"
          variant="ghost"
          aria-label="Show fields"
          aria-expanded={false}
          className="h-24 w-full rounded-none text-xs"
          onClick={() => toggle(true)}
        >
          <span className="rotate-180 [writing-mode:vertical-rl]">Fields</span>
        </Button>
      </aside>
    )
  }

  const problem = facets.isError ? facetsErrorText(facets.error) : null
  const loading = rows.length === 0 && (names.isLoading || facets.isLoading)
  return (
    <aside
      aria-label="Fields"
      className={cn(
        "flex min-h-0 w-[248px] shrink-0 flex-col border-r border-border text-[13px]",
        asideClass
      )}
    >
      <div className="flex items-start justify-between gap-1 px-4 pt-3 pb-2">
        <div className="flex min-w-0 flex-col">
          <h2 className="m-0 text-[13px] font-medium">Fields</h2>
          <p className="m-0 text-xs text-muted-foreground">
            {loading
              ? "Reading the fields"
              : fieldsNote(type, datasource.index, rows.length)}
          </p>
        </div>
        <Button
          type="button"
          variant="ghost"
          size="icon-xs"
          aria-label="Hide fields"
          aria-expanded
          onClick={() => toggle(false)}
        >
          <PanelLeftCloseIcon aria-hidden="true" />
        </Button>
      </div>
      <div className="px-4 pb-2">
        <Input
          aria-label="Search field names"
          placeholder="Search field names"
          value={filterText}
          className="h-7 text-xs"
          onChange={(event) => setFilterText(event.target.value)}
        />
      </div>
      <div className="flex min-h-0 flex-1 flex-col gap-1 overflow-y-auto px-3 pb-3">
        {problem === null ? null : (
          <Alert variant="destructive">
            <AlertDescription className="flex flex-col items-start gap-1 text-xs">
              {problem.text}
              {problem.retry ? (
                <Button
                  type="button"
                  size="xs"
                  variant="outline"
                  onClick={() => void facets.refetch()}
                >
                  Retry
                </Button>
              ) : null}
            </AlertDescription>
          </Alert>
        )}
        {loading ? (
          <div aria-busy="true" className="flex flex-col gap-1">
            {[0, 1, 2, 3, 4].map((item) => (
              <Skeleton key={item} className="h-6" />
            ))}
          </div>
        ) : (
          <>
            <FieldGroup
              title="Selected"
              rows={groups.selected}
              openField={openField}
              setOpenField={setOpenField}
              base={base}
              filter={filter}
              apply={apply}
            />
            <FieldGroup
              title="Available"
              rows={groups.available}
              openField={openField}
              setOpenField={setOpenField}
              base={base}
              filter={filter}
              apply={apply}
            />
            {rows.length === 0 ? (
              <p className="m-0 px-1.5 text-xs text-muted-foreground">
                No fields in this range.
              </p>
            ) : null}
          </>
        )}
      </div>
    </aside>
  )
}
