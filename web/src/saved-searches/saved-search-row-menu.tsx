import { useRef, useState } from "react"
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
import { moveChoices, moveLabel } from "@/folders/move-targets"
import { foldersQuery } from "@/folders/queries"
import { loadProblemText } from "@/service-accounts/queries"

import { DeleteSavedSearchDialog } from "./delete-saved-search-dialog"
import { useMoveSavedSearch, type SavedSearch } from "./queries"
import { RenameSavedSearchDialog } from "./rename-saved-search-dialog"

function focusPageHeading() {
  const heading = document.querySelector<HTMLElement>("main h1")
  if (heading === null) return
  if (!heading.hasAttribute("tabindex")) heading.tabIndex = -1
  heading.focus()
}

export function SavedSearchRowMenu({ saved }: { saved: SavedSearch }) {
  const folders = useQuery(foldersQuery()).data ?? []
  const move = useMoveSavedSearch(saved.id)
  const trigger = useRef<HTMLButtonElement>(null)
  const leaveFocusAlone = useRef(false)
  const [renaming, setRenaming] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const current = saved.folder.id
  const publishing = saved.folder.personal
  const choices = moveChoices(folders, current)

  if (!saved.permissions.edit) return null

  function moveTo(id: string) {
    const target = choices.find((choice) => choice.id === id)
    if (target === undefined || target.id === current) return
    leaveFocusAlone.current = true
    move.mutate(target.id, {
      onSuccess: () => {
        toast.success(
          `${publishing ? "Published" : "Moved"} to ${target.title}`
        )
        focusPageHeading()
      },
      onError: (error) => {
        leaveFocusAlone.current = false
        toast.error(`Could not move ${saved.title}. ${loadProblemText(error)}`)
      },
    })
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
              aria-label={`More for ${saved.title}`}
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
          <DropdownMenuItem
            onClick={() => {
              leaveFocusAlone.current = true
              setRenaming(true)
            }}
          >
            Rename...
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuGroup>
            <DropdownMenuLabel>{moveLabel(publishing)}</DropdownMenuLabel>
            <DropdownMenuRadioGroup value={current} onValueChange={moveTo}>
              {choices.map((choice) => (
                <DropdownMenuRadioItem
                  closeOnClick
                  key={choice.id}
                  value={choice.id}
                >
                  {choice.title}
                  {choice.id === current ? (
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
            variant="destructive"
            onClick={() => {
              leaveFocusAlone.current = true
              setDeleting(true)
            }}
          >
            Delete...
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <RenameSavedSearchDialog
        saved={saved}
        open={renaming}
        onOpenChange={setRenaming}
        finalFocus={() => trigger.current ?? true}
      />
      <DeleteSavedSearchDialog
        saved={saved}
        open={deleting}
        onOpenChange={setDeleting}
        onDeleted={focusPageHeading}
        finalFocus={() =>
          trigger.current?.isConnected ? trigger.current : false
        }
      />
    </>
  )
}
