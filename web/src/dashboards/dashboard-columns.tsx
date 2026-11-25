import type { ReactNode } from "react"
import { Link } from "@tanstack/react-router"
import { ArrowUpRightIcon } from "lucide-react"

import type { paths } from "@/api/schema.gen"
import type { DataGridColumn } from "@/components/data-grid/data-grid"
import {
  dataCell,
  type SkeletonRow,
} from "@/components/data-grid/skeleton-cell"
import { RelativeTime } from "@/components/relative-time"
import type { FolderSection } from "./folder-groups"
import { HealthCell } from "@/health/health-cell"
import { madeByChannel, madeByDetail } from "@/lib/origin"
import { cn } from "@/lib/utils"
import { dashboardLink } from "@/lib/links"

import { RowSelectBox, TitleHeader } from "./list-selection"
import { DashboardRowMenu } from "./row-menu"

export type DashboardSummary =
  paths["/dashboards"]["get"]["responses"][200]["content"]["application/json"]["dashboards"][number]

export type DashboardRow =
  | { kind: "dashboard"; id: string; dashboard: DashboardSummary }
  | {
      kind: "folder"
      id: string
      section: FolderSection
      collapsed: boolean
      countText: string | null
    }
  | { kind: "empty"; id: string }
  | SkeletonRow

const widths = {
  title: 220,
  health: 160,
  madeBy: 240,
  datasources: 144,
  openComments: 128,
  version: 80,
  views: 80,
  updated: 112,
  folder: 160,
  actions: 56,
}

const widthSlack = 8
const dropOrder = [
  "health",
  "madeBy",
  "version",
  "views",
  "datasources",
  "openComments",
] as const

type Droppable = (typeof dropOrder)[number]

const alwaysShown = widths.title + widths.updated + widths.actions

function fitsFrom(id: Droppable): number {
  const kept = dropOrder.slice(0, dropOrder.indexOf(id) + 1)
  return (
    alwaysShown +
    kept.reduce((total, key) => total + widths[key], 0) +
    widthSlack
  )
}

export const folderColumnWidth = widths.folder

const cell = (
  skeletonClassName: string,
  render: (dashboard: DashboardSummary) => ReactNode
) =>
  dataCell<DashboardRow, "dashboard">(
    "dashboard",
    skeletonClassName,
    ({ dashboard }) => render(dashboard)
  )

const rowHover =
  "[.ag-row-hover_&]:opacity-100 [[data-narrow-row]:hover_&]:opacity-100"

const zeroWidthAfterLastWord = "-my-1 -mr-6"

export const dashboardColumns: DataGridColumn<DashboardRow>[] = [
  {
    id: "title",
    header: "Title",
    headerCell: () => <TitleHeader />,
    minWidth: widths.title,
    flex: 1,
    autoHeight: true,
    cell: cell("h-5 w-48 max-w-full", (dashboard) => (
      <div className="relative min-w-0 py-2.5 pl-[30px] narrow:pl-0">
        <RowSelectBox dashboard={dashboard} />
        <Link
          {...dashboardLink(dashboard.id, dashboard.title)}
          className="inline rounded-sm font-medium wrap-anywhere no-underline outline-none hover:text-muted-foreground focus-visible:ring-2 focus-visible:ring-ring"
        >
          {dashboard.title}
        </Link>
        <Link
          {...dashboardLink(dashboard.id, dashboard.title)}
          target="_blank"
          rel="noopener"
          tabIndex={-1}
          aria-label={`Open ${dashboard.title} in a new tab`}
          title={`Open ${dashboard.title} in a new tab`}
          className={cn(
            zeroWidthAfterLastWord,
            "inline-flex size-6 items-center justify-center rounded-sm align-middle text-muted-foreground opacity-0 outline-none hover:text-foreground focus-visible:opacity-100 focus-visible:ring-2 focus-visible:ring-ring [@media(hover:none)]:opacity-100",
            rowHover
          )}
        >
          <ArrowUpRightIcon aria-hidden="true" className="size-3.5" />
        </Link>
        {dashboard.description ? (
          <div
            title={dashboard.description}
            className="line-clamp-1 text-xs wrap-anywhere text-muted-foreground"
          >
            {dashboard.description}
          </div>
        ) : null}
      </div>
    )),
  },
  {
    id: "health",
    header: "Health",
    width: widths.health,
    hideBelow: fitsFrom("health"),
    autoHeight: true,
    cell: cell("h-5 w-20", (dashboard) => (
      <HealthCell dashboardId={dashboard.id} title={dashboard.title} />
    )),
  },
  {
    id: "madeBy",
    header: "Made by",
    width: widths.madeBy,
    hideBelow: fitsFrom("madeBy"),
    cell: cell("h-5 w-28", (dashboard) => {
      const { creator } = dashboard
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
    }),
  },
  {
    id: "datasources",
    header: "Datasources",
    width: widths.datasources,
    hideBelow: fitsFrom("datasources"),
    cell: cell("h-5 w-16", (dashboard) =>
      dashboard.datasource_uids.length > 0 ? (
        <span className="truncate font-mono text-xs">
          {dashboard.datasource_uids.join(", ")}
        </span>
      ) : null
    ),
  },
  {
    id: "openComments",
    header: "Open comments",
    width: widths.openComments,
    hideBelow: fitsFrom("openComments"),
    cell: cell("h-5 w-10", (dashboard) =>
      dashboard.open_comment_count > 0 ? (
        <span className="tabular-nums">{dashboard.open_comment_count}</span>
      ) : null
    ),
  },
  {
    id: "version",
    header: "Version",
    width: widths.version,
    hideBelow: fitsFrom("version"),
    cell: cell("h-5 w-8", (dashboard) => (
      <span className="tabular-nums">{`v${dashboard.latest_version}`}</span>
    )),
  },
  {
    id: "views",
    header: "Views",
    width: widths.views,
    hideBelow: fitsFrom("views"),
    cell: cell("h-5 w-8", (dashboard) => (
      <span
        title="People who opened it, counted once for each day, over the last 30 days"
        className="tabular-nums"
      >
        {dashboard.view_count}
      </span>
    )),
  },
  {
    id: "updated",
    header: "Updated",
    width: widths.updated,
    align: "right",
    cell: cell("ml-auto h-5 w-16", (dashboard) => (
      <span className="whitespace-nowrap text-muted-foreground">
        <RelativeTime iso={dashboard.updated_at} />
      </span>
    )),
  },
]

export const folderColumn: DataGridColumn<DashboardRow> = {
  id: "folder",
  header: "Folder",
  width: widths.folder,
  autoHeight: true,
  cell: cell("h-5 w-24", (dashboard) => (
    <span className="wrap-anywhere">{dashboard.folder.title}</span>
  )),
}

export const actionsColumn: DataGridColumn<DashboardRow> = {
  id: "actions",
  header: "Actions",
  headerCell: () => <span className="sr-only">Actions</span>,
  width: widths.actions,
  action: true,
  cell: cell("ml-auto h-5 w-6", (dashboard) => (
    <DashboardRowMenu dashboard={dashboard} />
  )),
}
