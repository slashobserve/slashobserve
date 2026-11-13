import { Link } from "@tanstack/react-router"
import {
  ChevronDownIcon,
  ChevronRightIcon,
  FolderLockIcon,
  LockIcon,
  UsersIcon,
} from "lucide-react"

import { accessSummary, yourAccessText } from "@/dashboards/folder-groups"
import { Button } from "@/components/ui/button"

import type { Folder } from "./queries"

type Props = {
  folder: Folder
  collapsed: boolean
  countText: string | null
  columnCount: number
  onToggle: () => void
  continued?: boolean
}

export function FolderHeaderRow({
  folder,
  collapsed,
  countText,
  columnCount,
  onToggle,
  continued = false,
}: Props) {
  const AccessIcon = folder.personal
    ? FolderLockIcon
    : folder.restricted
      ? LockIcon
      : UsersIcon
  const yourAccess = yourAccessText(folder)
  return (
    <tr>
      <th
        scope="rowgroup"
        colSpan={columnCount}
        className="border-b px-3.5 py-2.5 text-left font-normal"
      >
        <span className="flex flex-wrap items-center gap-x-2.5 gap-y-1.5">
          <Button
            variant="ghost"
            size="icon-sm"
            aria-expanded={!collapsed}
            aria-label={`${collapsed ? "Expand" : "Collapse"} ${folder.title}`}
            onClick={onToggle}
          >
            {collapsed ? (
              <ChevronRightIcon aria-hidden="true" />
            ) : (
              <ChevronDownIcon aria-hidden="true" />
            )}
          </Button>
          <span className="font-semibold wrap-anywhere">{folder.title}</span>
          {countText === null ? null : (
            <span className="text-xs text-muted-foreground">{countText}</span>
          )}
          {continued ? (
            <span className="text-xs text-muted-foreground">
              Continued from the previous page
            </span>
          ) : null}
          <span className="inline-flex items-center gap-1.5 text-xs text-muted-foreground">
            <AccessIcon aria-hidden="true" className="size-3.5 flex-none" />
            {accessSummary(folder)}
          </span>
          {yourAccess === null ? null : (
            <span className="text-xs text-muted-foreground">{yourAccess}</span>
          )}
          {folder.permissions.manage ? (
            <Link
              to="/f/$folderId"
              params={{ folderId: folder.id }}
              search={{ settings: "1" }}
              className="ml-auto rounded-sm text-xs outline-none hover:text-muted-foreground focus-visible:ring-2 focus-visible:ring-ring"
            >
              Folder settings
            </Link>
          ) : null}
        </span>
      </th>
    </tr>
  )
}
