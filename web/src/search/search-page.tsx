import { useState, type FormEvent } from "react"
import { useInfiniteQuery } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"
import {
  BookmarkIcon,
  FlagIcon,
  LayoutDashboardIcon,
  MessageSquareIcon,
  NotebookTextIcon,
  type LucideIcon,
} from "lucide-react"

import { ApiError } from "@/api/errors"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { EmptyState } from "@/components/empty-state"
import { FilterToolbar } from "@/components/filter-toolbar"
import { LoadProblem } from "@/components/load-problem"
import { PageHeader } from "@/components/page-header"
import { RelativeTime } from "@/components/relative-time"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { useDocumentTitle } from "@/lib/document-title"
import { cn } from "@/lib/utils"
import {
  serverEntry,
  type MatchSegment,
  type SearchResult,
} from "@/shell/palette-items"

import { plainSegments } from "./plain-text"
import { searchKinds, searchPageQuery, type SearchKind } from "./queries"

const icons: Record<SearchKind, LucideIcon> = {
  dashboard: LayoutDashboardIcon,
  observation: NotebookTextIcon,
  comment: MessageSquareIcon,
  annotation: FlagIcon,
  saved_search: BookmarkIcon,
}

const kindLabels = Object.fromEntries(
  searchKinds.map(({ kind, label }) => [kind, label])
) as Record<SearchKind, string>

const rowClass =
  "flex w-full items-start gap-3 px-4 py-3 text-left no-underline outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring"

function Segments({ segments }: { segments: readonly MatchSegment[] }) {
  return (
    <>
      {segments.map((segment, index) =>
        segment.match ? (
          <mark
            key={index}
            className="bg-transparent font-medium text-foreground"
          >
            {segment.text}
          </mark>
        ) : (
          <span key={index}>{segment.text}</span>
        )
      )}
    </>
  )
}

function ResultRow({ result }: { result: SearchResult }) {
  const Icon = icons[result.kind]
  const linked = result.url === null ? null : serverEntry(result, result.url)
  const label = linked?.label ?? result.title
  const snippet = plainSegments(result.snippet)
  const body = (
    <>
      <Icon
        aria-hidden="true"
        className="mt-0.5 size-4 flex-none text-muted-foreground"
        strokeWidth={1.8}
      />
      <span className="min-w-0 flex-1">
        <span className="block truncate text-[13px] font-medium">{label}</span>
        {snippet.length > 0 ? (
          <span className="mt-0.5 line-clamp-2 block text-xs text-muted-foreground">
            <Segments segments={snippet} />
          </span>
        ) : null}
        <span className="mt-0.5 flex flex-wrap gap-x-2 text-xs text-muted-foreground">
          <span>{kindLabels[result.kind]}</span>
          {result.folder ? <span>{result.folder.title}</span> : null}
          {result.url === null ? (
            <span>On every dashboard, no page of its own</span>
          ) : null}
        </span>
      </span>
      <span className="flex-none text-xs text-muted-foreground">
        <RelativeTime iso={result.updated_at} />
      </span>
    </>
  )
  return linked === null ? (
    <div className={rowClass}>{body}</div>
  ) : (
    <Link to={linked.path} className={cn(rowClass, "hover:bg-muted")}>
      {body}
    </Link>
  )
}

function SearchForm({
  q,
  onSearch,
}: {
  q: string
  onSearch: (q: string) => void
}) {
  const [text, setText] = useState(q)
  function submit(event: FormEvent) {
    event.preventDefault()
    onSearch(text.trim())
  }
  return (
    <form role="search" onSubmit={submit} className="mb-3 flex gap-2">
      <Input
        type="search"
        aria-label="Search the workspace"
        placeholder="Search dashboards, observations, comments, annotations and saved searches"
        value={text}
        maxLength={200}
        autoFocus
        onChange={(event) => setText(event.target.value)}
      />
      <Button type="submit">Search</Button>
    </form>
  )
}

function Results({ q, kind }: { q: string; kind: SearchKind | null }) {
  const query = useInfiniteQuery(searchPageQuery(q, kind))
  if (query.isPending) {
    return (
      <div aria-busy="true" className="flex flex-col gap-2">
        <Skeleton className="h-14" />
        <Skeleton className="h-14" />
        <Skeleton className="h-14" />
      </div>
    )
  }
  if (query.isError) {
    if (query.error instanceof ApiError && query.error.status === 400) {
      return (
        <Alert variant="destructive">
          <AlertTitle>That search cannot run</AlertTitle>
          <AlertDescription>{query.error.message}</AlertDescription>
        </Alert>
      )
    }
    return (
      <LoadProblem
        title="Could not search"
        detail="The search did not answer. Try again."
        retrying={query.isFetching}
        onRetry={() => void query.refetch()}
      />
    )
  }
  const results = query.data.pages.flatMap((page) => page.results)
  if (results.length === 0) {
    return (
      <EmptyState title={`Nothing matches "${q}".`} docs="use/search">
        Search finds whole words and their English stems, not misspellings. Try
        fewer words, the start of a word, or all kinds instead of one.
      </EmptyState>
    )
  }
  return (
    <>
      <ul className="divide-y overflow-hidden rounded-card border bg-card">
        {results.map((result) => (
          <li key={`${result.kind}:${result.id}`}>
            <ResultRow result={result} />
          </li>
        ))}
      </ul>
      <div className="mt-3 flex items-center gap-3">
        <p role="status" className="text-xs text-muted-foreground">
          {results.length} {results.length === 1 ? "result" : "results"}
          {query.hasNextPage ? " so far" : ""}
        </p>
        {query.hasNextPage ? (
          <Button
            variant="outline"
            size="sm"
            disabled={query.isFetchingNextPage}
            focusableWhenDisabled
            onClick={() => void query.fetchNextPage()}
          >
            Show more
          </Button>
        ) : null}
      </div>
    </>
  )
}

export function SearchPage({
  q,
  kind,
  onChange,
}: {
  q: string
  kind: SearchKind | null
  onChange: (next: { q: string; kind: SearchKind | null }) => void
}) {
  useDocumentTitle(
    q === "" ? "Search - SlashObserve" : `${q} - Search - SlashObserve`
  )
  return (
    <>
      <PageHeader title="Search" />
      <SearchForm
        key={q}
        q={q}
        onSearch={(next) => onChange({ q: next, kind })}
      />
      {q === "" ? (
        <EmptyState title="Search the workspace" docs="use/search">
          Type some words to find dashboards, observations, comments,
          annotations and saved searches you can open. Quote words to match a
          phrase, put or between words, and put a minus before a word to leave
          it out.
        </EmptyState>
      ) : (
        <>
          <FilterToolbar label="Kind of result">
            <Button
              variant={kind === null ? "secondary" : "outline"}
              size="sm"
              aria-pressed={kind === null}
              onClick={() => onChange({ q, kind: null })}
            >
              All
            </Button>
            {searchKinds.map((entry) => (
              <Button
                key={entry.kind}
                variant={kind === entry.kind ? "secondary" : "outline"}
                size="sm"
                aria-pressed={kind === entry.kind}
                onClick={() => onChange({ q, kind: entry.kind })}
              >
                {entry.label}
              </Button>
            ))}
          </FilterToolbar>
          <Results q={q} kind={kind} />
        </>
      )}
    </>
  )
}
