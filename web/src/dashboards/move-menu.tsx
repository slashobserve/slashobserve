import { useEffect, useRef, useState } from "react"
import { useQuery } from "@tanstack/react-query"
import { EllipsisIcon } from "lucide-react"
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
import { foldersQuery } from "@/folders/queries"
import { loadProblemText } from "@/service-accounts/queries"

import type { DashboardSummary } from "./dashboard-columns"
import { useMoveDashboard } from "./queries"

type Target = { id: string; title: string }

export function MoveMenu({ dashboard }: { dashboard: DashboardSummary }) {
  const folders = useQuery(foldersQuery()).data ?? []
  const move = useMoveDashboard()
  const trigger = useRef<HTMLButtonElement>(null)
  const refocus = useRef(false)
  const leaveFocusAlone = useRef(false)
  const [creating, setCreating] = useState(false)
  const current = dashboard.folder.id
  const choices = folders.filter(
    (folder) => folder.permissions.edit || folder.id === current
  )

  useEffect(() => {
    if (!refocus.current) return
    refocus.current = false
    trigger.current?.focus()
  }, [current])

  function moveTo(target: Target) {
    refocus.current = true
    move.mutate(
      { id: dashboard.id, folderId: target.id },
      {
        onSuccess: () => toast.success(`Moved to ${target.title}`),
        onError: (error) => {
          refocus.current = false
          toast.error(
            `Could not move ${dashboard.title}. ${loadProblemText(error)}`
          )
        },
      }
    )
  }

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
          <DropdownMenuGroup>
            <DropdownMenuLabel>Move to folder</DropdownMenuLabel>
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
          <DropdownMenuSeparator />
          <DropdownMenuItem
            onClick={() => {
              leaveFocusAlone.current = true
              setCreating(true)
            }}
          >
            New folder
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <NewFolderDialog
        open={creating}
        onOpenChange={setCreating}
        onCreated={moveTo}
        finalFocus={trigger}
      />
    </>
  )
}
