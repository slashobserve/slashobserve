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
import { madeByLabel } from "@/lib/origin"

import { MoveMenu } from "./move-menu"

declare module "@tanstack/react-table" {
  /* eslint-disable @typescript-eslint/no-unused-vars -- a merged interface must repeat the library's type parameters */
  interface ColumnMeta<
    TFeatures extends TableFeatures,
    TData extends RowData,
    TValue extends CellData = CellData,
  > {
    headerClassName?: string
    cellClassName?: string
    skeletonClassName: string
  }
  /* eslint-enable @typescript-eslint/no-unused-vars */
}

export type DashboardSummary =
  paths["/dashboards"]["get"]["responses"][200]["content"]["application/json"]["dashboards"][number]

export const listFeatures = tableFeatures({})

const column = createColumnHelper<typeof listFeatures, DashboardSummary>()

export const dashboardColumns = column.columns([
  column.display({
    id: "title",
    header: "Title",
    cell: ({ row }) => (
      <>
        <Link
          to="/dashboards/$dashboardId"
          params={{ dashboardId: row.original.id }}
          className="inline rounded-sm font-medium wrap-anywhere no-underline outline-none hover:text-muted-foreground focus-visible:ring-2 focus-visible:ring-ring"
        >
          {row.original.title}
        </Link>
        <Link
          to="/dashboards/$dashboardId"
          params={{ dashboardId: row.original.id }}
          target="_blank"
          rel="noopener"
          aria-label={`Open ${row.original.title} in a new tab`}
          title={`Open ${row.original.title} in a new tab`}
          className="ml-1 inline-block rounded-sm align-text-bottom text-muted-foreground opacity-0 outline-none group-hover/row:opacity-100 hover:text-foreground focus-visible:opacity-100 focus-visible:ring-2 focus-visible:ring-ring [@media(hover:none)]:opacity-100"
        >
          <ArrowUpRightIcon aria-hidden="true" className="size-3.5" />
        </Link>
        {row.original.description ? (
          <div className="text-xs wrap-anywhere text-muted-foreground">
            {row.original.description}
          </div>
        ) : null}
      </>
    ),
    meta: {
      cellClassName: "pl-11",
      skeletonClassName: "h-5 w-48 max-w-full",
    },
  }),
  column.display({
    id: "madeBy",
    header: "Made by",
    cell: ({ row }) => madeByLabel(row.original.creator),
    meta: {
      cellClassName: "min-w-40 text-muted-foreground",
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
      ) : (
        <span className="text-muted-foreground">None</span>
      ),
    meta: { skeletonClassName: "h-5 w-16" },
  }),
  column.display({
    id: "openComments",
    header: "Open comments",
    cell: ({ row }) =>
      row.original.open_comment_count > 0
        ? row.original.open_comment_count
        : "None",
    meta: { cellClassName: "tabular-nums", skeletonClassName: "h-5 w-10" },
  }),
  column.display({
    id: "version",
    header: "Version",
    cell: ({ row }) => `v${row.original.latest_version}`,
    meta: { cellClassName: "tabular-nums", skeletonClassName: "h-5 w-8" },
  }),
  column.display({
    id: "updated",
    header: "Updated",
    cell: ({ row }) => <RelativeTime iso={row.original.updated_at} />,
    meta: {
      headerClassName: "text-right",
      cellClassName: "text-right whitespace-nowrap text-muted-foreground",
      skeletonClassName: "ml-auto h-5 w-16",
    },
  }),
])

export const actionsColumn = column.display({
  id: "actions",
  header: () => <span className="sr-only">Actions</span>,
  cell: ({ row }) =>
    row.original.permissions.edit ? (
      <MoveMenu dashboard={row.original} />
    ) : null,
  meta: {
    headerClassName: "w-10",
    cellClassName: "w-10 text-right",
    skeletonClassName: "ml-auto h-5 w-6",
  },
})
