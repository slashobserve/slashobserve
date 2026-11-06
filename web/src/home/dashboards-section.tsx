import { useEffect, useId, useMemo, useState, type ReactNode } from "react"
import { Link, useNavigate } from "@tanstack/react-router"
import { SearchIcon } from "lucide-react"

import type { components } from "@/api/schema.gen"
import { RelativeTime } from "@/components/relative-time"
import { Button } from "@/components/ui/button"
import {
  Combobox,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxInput,
  ComboboxItem,
  ComboboxList,
} from "@/components/ui/combobox"
import { InputGroupAddon } from "@/components/ui/input-group"
import { Skeleton } from "@/components/ui/skeleton"
import { dashboardLink } from "@/lib/links"

import { useDashboardSearch, useFirstDashboards, useStars } from "./queries"
import { recentToShow, useRecentDashboards } from "./recent"
import { isEmptySearch, parseHomeQuery, rankResults } from "./search"

type ListItem = components["schemas"]["DashboardListItem"]

type Row = { id: string; title: string; detail: ReactNode }

const recentShown = 5
const searchDelayMs = 200

export function DashboardsSection() {
  const headingId = useId()
  const stars = useStars()
  const recent = useRecentDashboards()
  const starredIds = useMemo(
    () => new Set((stars.data ?? []).map((star) => star.id)),
    [stars.data]
  )
  const recentRows = recentToShow(recent, starredIds, recentShown)
  const nothingPersonal = stars.data?.length === 0 && recent.length === 0
  const first = useFirstDashboards(nothingPersonal)

  if (nothingPersonal && first.data?.length === 0) return null

  return (
    <section aria-labelledby={headingId} className="mt-7 w-full max-w-[720px]">
      <div className="flex items-center justify-between px-0.5 pb-2">
        <h2
          id={headingId}
          className="text-[13px] font-medium text-muted-foreground"
        >
          Dashboards
        </h2>
        <Link
          to="/dashboards"
          className="rounded-control text-xs text-muted-foreground outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
        >
          Browse all
        </Link>
      </div>
      <div className="flex flex-col gap-3">
        <DashboardSearch />
        {stars.isPending ? (
          <PendingRows />
        ) : stars.isError ? (
          <div className="flex items-center gap-3 rounded-card border bg-background px-3.5 py-2.5">
            <p className="text-xs text-muted-foreground">
              Could not load starred dashboards.
            </p>
            <Button
              variant="outline"
              size="xs"
              disabled={stars.isFetching}
              focusableWhenDisabled
              onClick={() => void stars.refetch()}
            >
              Retry
            </Button>
          </div>
        ) : stars.data.length > 0 ? (
          <DashboardRows
            label="Starred"
            rows={stars.data.map((star) => ({
              id: star.id,
              title: star.title,
              detail: star.folder_title,
            }))}
          />
        ) : null}
        {recentRows.length > 0 ? (
          <DashboardRows
            label="Recently viewed"
            rows={recentRows.map((entry) => ({
              id: entry.id,
              title: entry.title,
              detail: <RelativeTime iso={entry.viewed_at} />,
            }))}
          />
        ) : null}
        {!nothingPersonal ? null : first.isPending ? (
          <PendingRows />
        ) : first.data === undefined ? null : (
          <DashboardRows
            label="Suggested"
            rows={first.data.map((dashboard) => ({
              id: dashboard.id,
              title: dashboard.title,
              detail: dashboard.folder.title,
            }))}
          />
        )}
      </div>
    </section>
  )
}

function PendingRows() {
  return (
    <ul
      aria-busy="true"
      className="overflow-hidden rounded-card border bg-background"
    >
      {Array.from({ length: 3 }, (_, index) => (
        <li key={index} className="border-b last:border-b-0">
          <Skeleton className="h-[39px] rounded-none" />
        </li>
      ))}
    </ul>
  )
}

function DashboardRows({ label, rows }: { label: string; rows: Row[] }) {
  const labelId = useId()
  return (
    <div>
      <h3 id={labelId} className="px-0.5 pb-1.5 text-xs text-muted-foreground">
        {label}
      </h3>
      <ul
        aria-labelledby={labelId}
        className="overflow-hidden rounded-card border bg-background"
      >
        {rows.map((row) => (
          <li key={row.id} className="border-b last:border-b-0">
            <Link
              {...dashboardLink(row.id, row.title)}
              className="flex w-full items-center gap-3 px-3.5 py-2.5 outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset"
            >
              <span className="min-w-0 flex-1 truncate text-[13px]">
                {row.title}
              </span>
              <span className="max-w-[200px] flex-none truncate text-right text-xs text-muted-foreground">
                {row.detail}
              </span>
            </Link>
          </li>
        ))}
      </ul>
    </div>
  )
}

function useSettledText(text: string): string {
  const [settled, setSettled] = useState(text)
  useEffect(() => {
    const timer = setTimeout(() => setSettled(text), searchDelayMs)
    return () => clearTimeout(timer)
  }, [text])
  return settled
}

function DashboardSearch() {
  const navigate = useNavigate()
  const [text, setText] = useState("")
  const search = parseHomeQuery(useSettledText(text))
  const empty = isEmptySearch(search)
  const results = useDashboardSearch(search, !empty)
  const items = empty ? [] : rankResults(results.data ?? [], search.q)

  return (
    <Combobox<ListItem>
      items={items}
      value={null}
      inputValue={text}
      filter={null}
      autoHighlight
      itemToStringLabel={(item) => item.title}
      isItemEqualToValue={(item, value) => item.id === value.id}
      onInputValueChange={(next, details) =>
        setText(details.reason === "item-press" ? "" : next)
      }
      onValueChange={(item) => {
        if (item !== null) void navigate(dashboardLink(item.id, item.title))
      }}
    >
      <ComboboxInput
        aria-label="Search dashboards"
        placeholder="Search dashboards, like: tag:prod latency"
        showTrigger={false}
        className="w-full bg-background"
      >
        <InputGroupAddon align="inline-start">
          <SearchIcon aria-hidden="true" />
        </InputGroupAddon>
      </ComboboxInput>
      <ComboboxContent>
        <ComboboxEmpty>
          {empty
            ? "Type a title, or tag:name"
            : results.isFetching
              ? "Searching"
              : "No dashboards found"}
        </ComboboxEmpty>
        <ComboboxList>
          {(item: ListItem) => (
            <ComboboxItem key={item.id} value={item}>
              <span className="min-w-0 flex-1 truncate">{item.title}</span>
              <span className="max-w-[160px] flex-none truncate text-xs text-muted-foreground">
                {item.folder.title}
              </span>
            </ComboboxItem>
          )}
        </ComboboxList>
      </ComboboxContent>
    </Combobox>
  )
}
