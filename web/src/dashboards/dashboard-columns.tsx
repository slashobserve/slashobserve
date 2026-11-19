import { Link } from "@tanstack/react-router"
import {
  createColumnHelper,
  tableFeatures,
  type CellData,
  type RowData,
  type TableFeatures,
} from "@tanstack/react-table"
import { ArrowUpRightIcon } from "lucide-react"

import type { paths } from "@/api/schema.gen"
import { RelativeTime } from "@/components/relative-time"
import { HealthCell } from "@/health/health-cell"
import { madeByChannel, madeByDetail } from "@/lib/origin"
import { cn } from "@/lib/utils"
import { dashboardLink } from "@/lib/links"

import { DashboardRowMenu } from "./row-menu"

declare module "@tanstack/react-table" {
  /* eslint-disable @typescript-eslint/no-unused-vars -- a merged interface must repeat the library's type parameters */
  interface ColumnMeta<
    TFeatures extends TableFeatures,
    TData extends RowData,
    TValue extends CellData = CellData,
  > {
    headerClassName?: string
    cellClassName?: string
    stackedLabel?: string
    skeletonClassName: string
  }
  /* eslint-enable @typescript-eslint/no-unused-vars */
}

export type DashboardSummary =
  paths["/dashboards"]["get"]["responses"][200]["content"]["application/json"]["dashboards"][number]

const hiddenWhenCramped = "@max-[1000px]:not-narrow:hidden"

const titleMinWidth = "min-w-60"

const columnWidth = {
  health: "w-40",
  madeBy: "w-60",
  datasources: "w-36",
  openComments: "w-32",
  version: "w-20",
  views: "w-20",
  updated: "w-28",
  folder: "w-40",
}

const zeroWidthAfterLastWord = "-my-1 -mr-6"

export const listFeatures = tableFeatures({})

const column = createColumnHelper<typeof listFeatures, DashboardSummary>()

export const dashboardColumns = column.columns([
  column.display({
    id: "title",
    header: "Title",
    cell: ({ row }) => (
      <>
        <Link
          {...dashboardLink(row.original.id, row.original.title)}
          className="inline rounded-sm font-medium wrap-anywhere no-underline outline-none hover:text-muted-foreground focus-visible:ring-2 focus-visible:ring-ring"
        >
          {row.original.title}
        </Link>
        <Link
          {...dashboardLink(row.original.id, row.original.title)}
          target="_blank"
          rel="noopener"
          tabIndex={-1}
          aria-label={`Open ${row.original.title} in a new tab`}
          title={`Open ${row.original.title} in a new tab`}
          className={cn(
            zeroWidthAfterLastWord,
            "inline-flex size-6 items-center justify-center rounded-sm align-middle text-muted-foreground opacity-0 outline-none group-hover/row:opacity-100 hover:text-foreground focus-visible:opacity-100 focus-visible:ring-2 focus-visible:ring-ring [@media(hover:none)]:opacity-100"
          )}
        >
          <ArrowUpRightIcon aria-hidden="true" className="size-3.5" />
        </Link>
        {row.original.description ? (
          <div
            title={row.original.description}
            className="line-clamp-1 text-xs wrap-anywhere text-muted-foreground"
          >
            {row.original.description}
          </div>
        ) : null}
      </>
    ),
    meta: {
      headerClassName: titleMinWidth,
      cellClassName: cn("pl-11", titleMinWidth),
      skeletonClassName: "h-5 w-48 max-w-full",
    },
  }),
  column.display({
    id: "health",
    header: "Health",
    cell: ({ row }) => (
      <HealthCell dashboardId={row.original.id} title={row.original.title} />
    ),
    meta: {
      headerClassName: columnWidth.health,
      cellClassName: cn("min-w-36", columnWidth.health),
      skeletonClassName: "h-5 w-20",
    },
  }),
  column.display({
    id: "madeBy",
    header: "Made by",
    cell: ({ row }) => {
      const { creator } = row.original
      const channel = madeByChannel(creator)
      const detail = madeByDetail(creator)
      return (
        <div
          title={
            detail === ""
              ? creator.actor.label
              : `${creator.actor.label} · ${detail}`
          }
          className="max-w-56 truncate"
        >
          {creator.actor.label}
          {channel === null ? null : (
            <span className="text-muted-foreground">{` · ${channel}`}</span>
          )}
        </div>
      )
    },
    meta: {
      headerClassName: columnWidth.madeBy,
      cellClassName: cn("min-w-40", columnWidth.madeBy),
      stackedLabel: "Made by",
      skeletonClassName: "h-5 w-28",
    },
  }),
  column.display({
    id: "datasources",
    header: "Datasources",
    cell: ({ row }) =>
      row.original.datasource_uids.length > 0 ? (
        <span className="font-mono text-xs">
          {row.original.datasource_uids.join(", ")}
        </span>
      ) : null,
    meta: {
      headerClassName: cn(hiddenWhenCramped, columnWidth.datasources),
      cellClassName: cn(hiddenWhenCramped, columnWidth.datasources),
      skeletonClassName: "h-5 w-16",
    },
  }),
  column.display({
    id: "openComments",
    header: "Open comments",
    cell: ({ row }) =>
      row.original.open_comment_count > 0
        ? row.original.open_comment_count
        : null,
    meta: {
      headerClassName: cn(hiddenWhenCramped, columnWidth.openComments),
      cellClassName: cn(
        "tabular-nums",
        hiddenWhenCramped,
        columnWidth.openComments
      ),
      stackedLabel: "Open comments",
      skeletonClassName: "h-5 w-10",
    },
  }),
  column.display({
    id: "version",
    header: "Version",
    cell: ({ row }) => `v${row.original.latest_version}`,
    meta: {
      headerClassName: columnWidth.version,
      cellClassName: cn("tabular-nums", columnWidth.version),
      skeletonClassName: "h-5 w-8",
    },
  }),
  column.display({
    id: "views",
    header: "Views",
    cell: ({ row }) => (
      <span title="People who opened it, counted once for each day, over the last 30 days">
        {row.original.view_count}
      </span>
    ),
    meta: {
      headerClassName: columnWidth.views,
      cellClassName: cn("tabular-nums", columnWidth.views),
      stackedLabel: "Views",
      skeletonClassName: "h-5 w-8",
    },
  }),
  column.display({
    id: "updated",
    header: "Updated",
    cell: ({ row }) => <RelativeTime iso={row.original.updated_at} />,
    meta: {
      headerClassName: cn("text-right", columnWidth.updated),
      cellClassName: cn(
        "text-right whitespace-nowrap text-muted-foreground",
        columnWidth.updated
      ),
      skeletonClassName: "ml-auto h-5 w-16",
    },
  }),
])

export const folderColumn = column.display({
  id: "folder",
  header: "Folder",
  cell: ({ row }) => (
    <span className="wrap-anywhere">{row.original.folder.title}</span>
  ),
  meta: {
    headerClassName: columnWidth.folder,
    cellClassName: columnWidth.folder,
    stackedLabel: "Folder",
    skeletonClassName: "h-5 w-24",
  },
})

export const actionsColumn = column.display({
  id: "actions",
  header: () => <span className="sr-only">Actions</span>,
  cell: ({ row }) => <DashboardRowMenu dashboard={row.original} />,
  meta: {
    headerClassName: "w-10",
    cellClassName: "w-10 text-right",
    skeletonClassName: "ml-auto h-5 w-6",
  },
})
