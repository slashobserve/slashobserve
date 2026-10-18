import { Link } from "@tanstack/react-router"

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button, buttonVariants } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { dashboardSearchFromBack } from "@/dashboards/search"
import { formatDateTime, formatRelative } from "@/lib/time"
import { cn } from "@/lib/utils"

import { RestoreVersionButton } from "./restore-version-button"
import { useVersionHistory, type VersionMeta } from "./use-version-history"

type Props = {
  dashboardId: string
  latestVersion: number
  back: string | undefined
  onCompare: (version: number) => void
  onRestored: () => void
}

const skeletonRows = 5
const rowLayout =
  "max-sm:flex max-sm:flex-wrap max-sm:items-center max-sm:gap-x-3 max-sm:gap-y-1.5 max-sm:px-2 max-sm:py-3"
const cellLayout = "max-sm:p-0"

function VersionRow({
  dashboardId,
  meta,
  latestVersion,
  back,
  onCompare,
  onRestored,
}: Props & { meta: VersionMeta }) {
  const latest = meta.version === latestVersion
  return (
    <TableRow className={rowLayout}>
      <TableCell className={cellLayout}>
        <div className="flex flex-wrap items-center gap-1.5">
          <span className="font-medium tabular-nums">v{meta.version}</span>
          {latest ? <Badge variant="secondary">Latest</Badge> : null}
          {meta.restored_from !== null ? (
            <Badge variant="outline">Restored from v{meta.restored_from}</Badge>
          ) : null}
        </div>
      </TableCell>
      <TableCell
        className={`${cellLayout} wrap-anywhere whitespace-normal max-sm:w-full`}
      >
        {meta.summary}
      </TableCell>
      <TableCell className={`${cellLayout} wrap-anywhere whitespace-normal`}>
        {meta.author.name}
      </TableCell>
      <TableCell className={`${cellLayout} text-muted-foreground`}>
        <time
          dateTime={meta.created_at}
          title={formatDateTime(meta.created_at)}
        >
          {formatRelative(meta.created_at)}
        </time>
      </TableCell>
      <TableCell className={`${cellLayout} max-sm:mt-1 max-sm:w-full`}>
        <div className="flex flex-wrap items-center gap-1.5 sm:flex-nowrap">
          <Link
            to="/dashboards/$dashboardId"
            params={{ dashboardId }}
            search={
              latest
                ? dashboardSearchFromBack(back, { dropVersion: true })
                : {
                    ...dashboardSearchFromBack(back, { dropVersion: true }),
                    version: meta.version,
                  }
            }
            aria-label={`View version ${meta.version}`}
            className={cn(
              buttonVariants({ variant: "ghost", size: "sm" }),
              "-ml-[11px]"
            )}
          >
            View
          </Link>
          <Button
            variant="outline"
            size="sm"
            aria-label={`Compare version ${meta.version} with ${
              meta.version === 1
                ? "an empty dashboard"
                : `version ${meta.version - 1}`
            }`}
            onClick={() => onCompare(meta.version)}
          >
            Compare
          </Button>
          {latest ? null : (
            <RestoreVersionButton
              dashboardId={dashboardId}
              version={meta.version}
              latestVersion={latestVersion}
              onRestored={onRestored}
            />
          )}
        </div>
      </TableCell>
    </TableRow>
  )
}

function SkeletonRows() {
  return Array.from({ length: skeletonRows }, (_, index) => (
    <TableRow key={index} className={`${rowLayout} hover:bg-transparent`}>
      <TableCell className={cellLayout}>
        <Skeleton className="h-5 w-10" />
      </TableCell>
      <TableCell className={`${cellLayout} max-sm:w-full`}>
        <Skeleton className="h-5 w-40 max-w-full" />
      </TableCell>
      <TableCell className={cellLayout}>
        <Skeleton className="h-5 w-20" />
      </TableCell>
      <TableCell className={cellLayout}>
        <Skeleton className="h-5 w-16" />
      </TableCell>
      <TableCell className={`${cellLayout} max-sm:w-full`}>
        <Skeleton className="h-7 w-32" />
      </TableCell>
    </TableRow>
  ))
}

export function VersionList({
  dashboardId,
  latestVersion,
  back,
  onCompare,
  onRestored,
}: Props) {
  const query = useVersionHistory(dashboardId)
  const versions = query.data?.pages.flatMap((page) => page.versions) ?? []
  const loadMoreFailed = query.isFetchNextPageError

  return (
    <section aria-label="Versions">
      {query.isError && !loadMoreFailed ? (
        <Alert variant="destructive" className="mb-4">
          <AlertTitle>Could not load versions</AlertTitle>
          <AlertDescription>
            <p>{query.error.message}</p>
            <Button
              variant="outline"
              size="sm"
              className="text-foreground"
              disabled={query.isFetching}
              focusableWhenDisabled
              aria-busy={query.isFetching}
              onClick={() => void query.refetch()}
            >
              Retry
            </Button>
          </AlertDescription>
        </Alert>
      ) : null}
      {query.isError && !query.data ? null : (
        <Table
          aria-label="Versions"
          aria-busy={query.isPending}
          className="max-sm:block max-sm:[&_tbody]:block max-sm:[&_thead]:hidden"
        >
          <TableHeader>
            <TableRow className="hover:bg-transparent">
              <TableHead>Version</TableHead>
              <TableHead>Summary</TableHead>
              <TableHead>Author</TableHead>
              <TableHead>Saved</TableHead>
              <TableHead>Actions</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {query.isPending ? (
              <SkeletonRows />
            ) : (
              versions.map((meta) => (
                <VersionRow
                  key={meta.version}
                  dashboardId={dashboardId}
                  meta={meta}
                  latestVersion={latestVersion}
                  back={back}
                  onCompare={onCompare}
                  onRestored={onRestored}
                />
              ))
            )}
          </TableBody>
        </Table>
      )}
      {query.hasNextPage ? (
        <div className="mt-4 flex flex-wrap items-center justify-center gap-x-3 gap-y-2">
          {loadMoreFailed ? (
            <p role="alert" className="text-sm text-destructive">
              Could not load more versions: {query.error?.message}
            </p>
          ) : null}
          <Button
            variant="outline"
            className="text-foreground"
            disabled={query.isFetchingNextPage}
            focusableWhenDisabled
            aria-busy={query.isFetchingNextPage}
            onClick={() => void query.fetchNextPage()}
          >
            Load more
          </Button>
        </div>
      ) : null}
    </section>
  )
}
