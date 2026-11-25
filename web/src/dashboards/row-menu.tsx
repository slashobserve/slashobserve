import { useEffect, useRef, useState } from "react"
import { useQuery } from "@tanstack/react-query"
import { useRouteContext } from "@tanstack/react-router"
import { CopyIcon, EllipsisIcon } from "lucide-react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { NewFolderDialog } from "@/folders/new-folder-dialog"
import { moveChoices, moveLabel } from "@/folders/move-targets"
import { foldersQuery } from "@/folders/queries"
import { roleAtLeast } from "@/lib/roles"
import { loadProblemText } from "@/service-accounts/queries"

import type { DashboardSummary } from "./dashboard-columns"
import { DeleteDashboardDialog } from "./delete-dashboard-dialog"
import { neighbourAfterDelete } from "./delete-dashboard"
import { duplicateTargets } from "./duplicate"
import { DuplicateDashboardDialog } from "./duplicate-dashboard-dialog"
import { mayEditBesidesContent } from "./permissions"
import { useMoveDashboard } from "./queries"

type Target = { id: string; title: string }

function gridOf(trigger: HTMLElement | null): ParentNode {
  return trigger?.closest('[role="grid"]') ?? document
}

function listedDashboardIds(grid: ParentNode): string[] {
  return Array.from(grid.querySelectorAll<HTMLElement>('[role="row"][row-id]'))
    .filter((row) => !row.getAttribute("row-id")?.includes(":"))
    .sort(
      (a, b) =>
        Number(a.getAttribute("row-index")) -
        Number(b.getAttribute("row-index"))
    )
    .map((row) => row.getAttribute("row-id") ?? "")
}

function focusNeighbour(neighbour: string | null, grid: ParentNode) {
  const target =
    neighbour === null
      ? document.querySelector<HTMLElement>("main h1")
      : grid.querySelector<HTMLElement>(`[role="row"][row-id="${neighbour}"] a`)
  if (target === null) return
  if (neighbour === null && !target.hasAttribute("tabindex")) {
    target.tabIndex = -1
  }
  target.focus()
}

export function DashboardRowMenu({
  dashboard,
}: {
  dashboard: DashboardSummary
}) {
  const { principal } = useRouteContext({ from: "/_authed" })
  const folders = useQuery(foldersQuery()).data ?? []
  const move = useMoveDashboard()
  const trigger = useRef<HTMLButtonElement>(null)
  const refocus = useRef(false)
  const leaveFocusAlone = useRef(false)
  const deleted = useRef(false)
  const neighbour = useRef<string | null>(null)
  const grid = useRef<ParentNode>(document)
  const [creating, setCreating] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [duplicating, setDuplicating] = useState(false)
  const current = dashboard.folder.id
  const canEdit = dashboard.permissions.edit
  const canDelete = mayEditBesidesContent(
    dashboard.permissions,
    dashboard.provisioned
  )
  const publishing = dashboard.folder.personal
  const movedVerb = publishing ? "Published" : "Moved"
  const targets = duplicateTargets(folders, current)
  const canDuplicate = targets.choices.length > 0
  const choices = moveChoices(folders, current)

  useEffect(() => {
    if (!refocus.current) return
    refocus.current = false
    trigger.current?.focus()
  }, [current])

  useEffect(
    () => () => {
      const active = document.activeElement
      if (refocus.current && (active === null || active === document.body)) {
        focusNeighbour(neighbour.current, grid.current)
      }
    },
    []
  )

  function moveTo(target: Target) {
    refocus.current = true
    grid.current = gridOf(trigger.current)
    neighbour.current = neighbourAfterDelete(
      listedDashboardIds(grid.current),
      dashboard.id
    )
    move.mutateAsync({ id: dashboard.id, folderId: target.id }).then(
      () => toast.success(`${movedVerb} to ${target.title}`),
      (error) => {
        refocus.current = false
        toast.error(
          `Could not move ${dashboard.title}. ${loadProblemText(error)}`
        )
      }
    )
  }

  function startDelete() {
    leaveFocusAlone.current = true
    deleted.current = false
    grid.current = gridOf(trigger.current)
    neighbour.current = neighbourAfterDelete(
      listedDashboardIds(grid.current),
      dashboard.id
    )
    setDeleting(true)
  }

  function finishDelete(alreadyGone: boolean) {
    deleted.current = true
    toast.success(
      alreadyGone
        ? `${dashboard.title} was already deleted`
        : `Deleted ${dashboard.title}`
    )
    setDeleting(false)
    focusNeighbour(neighbour.current, grid.current)
  }

  if (!canDelete && !canDuplicate) return null

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            <Button
              ref={trigger}
              variant="ghost"
              size="icon-sm"
              aria-label={`More for ${dashboard.title}`}
            />
          }
        >
          <EllipsisIcon aria-hidden="true" />
        </DropdownMenuTrigger>
        <DropdownMenuContent
          align="end"
          className="min-w-60 wrap-anywhere"
          finalFocus={() => {
            const leave = leaveFocusAlone.current
            leaveFocusAlone.current = false
            return !leave
          }}
        >
          {canDuplicate ? (
            <DropdownMenuItem
              onClick={() => {
                leaveFocusAlone.current = true
                setDuplicating(true)
              }}
            >
              <CopyIcon aria-hidden="true" />
              Duplicate...
            </DropdownMenuItem>
          ) : null}
          {canEdit ? (
            <>
              {canDuplicate ? <DropdownMenuSeparator /> : null}
              <DropdownMenuGroup>
                <DropdownMenuLabel>{moveLabel(publishing)}</DropdownMenuLabel>
                <DropdownMenuRadioGroup
                  value={current}
                  onValueChange={(id) => {
                    const target = choices.find((folder) => folder.id === id)
                    if (target !== undefined) moveTo(target)
                  }}
                >
                  {choices.map((folder) => (
                    <DropdownMenuRadioItem
                      closeOnClick
                      key={folder.id}
                      value={folder.id}
                    >
                      {folder.title}
                      {folder.id === current ? (
                        <span className="ml-auto text-xs text-muted-foreground">
                          Current
                        </span>
                      ) : null}
                    </DropdownMenuRadioItem>
                  ))}
                </DropdownMenuRadioGroup>
              </DropdownMenuGroup>
              {roleAtLeast(principal.role, "editor") ? (
                <>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem
                    onClick={() => {
                      leaveFocusAlone.current = true
                      setCreating(true)
                    }}
                  >
                    New folder
                  </DropdownMenuItem>
                </>
              ) : null}
            </>
          ) : null}
          {canDelete ? (
            <>
              {canEdit || canDuplicate ? <DropdownMenuSeparator /> : null}
              <DropdownMenuItem variant="destructive" onClick={startDelete}>
                Delete dashboard...
              </DropdownMenuItem>
            </>
          ) : null}
        </DropdownMenuContent>
      </DropdownMenu>
      <DuplicateDashboardDialog
        source={{
          id: dashboard.id,
          title: dashboard.title,
          version: dashboard.latest_version,
        }}
        targets={targets}
        open={duplicating}
        onOpenChange={setDuplicating}
        finalFocus={() => trigger.current ?? true}
      />
      <NewFolderDialog
        open={creating}
        onOpenChange={setCreating}
        onCreated={moveTo}
        finalFocus={trigger}
      />
      <DeleteDashboardDialog
        dashboard={{
          id: dashboard.id,
          title: dashboard.title,
          versions: dashboard.latest_version,
        }}
        open={deleting}
        onOpenChange={setDeleting}
        onDeleted={finishDelete}
        finalFocus={() => (deleted.current ? false : (trigger.current ?? true))}
      />
    </>
  )
}
