import { useQuery } from "@tanstack/react-query"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { moveChoices, moveLabel } from "@/folders/move-targets"
import { foldersQuery } from "@/folders/queries"
import { loadProblemText } from "@/service-accounts/queries"

import { useMoveFinding, type FindingEnvelope } from "./queries"

export function FindingMoveMenu({ envelope }: { envelope: FindingEnvelope }) {
  const { finding, folder, permissions } = envelope
  const folders = useQuery(foldersQuery()).data ?? []
  const move = useMoveFinding(finding.id)
  if (!permissions.edit || finding.dashboard_id !== null) return null
  const current = folder?.id ?? null
  const publishing = folder?.personal === true
  const choices = moveChoices(folders, current)

  function moveTo(id: string) {
    const target = choices.find((choice) => choice.id === id)
    if (target === undefined || target.id === current) return
    move.mutate(target.id, {
      onSuccess: () =>
        toast.success(
          `${publishing ? "Published" : "Moved"} to ${target.title}`
        ),
      onError: (error) =>
        toast.error(
          `Could not move ${finding.title}. ${loadProblemText(error)}`
        ),
    })
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button variant="outline" size="sm" disabled={move.isPending} />
        }
      >
        Move
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="min-w-60 wrap-anywhere">
        <DropdownMenuGroup>
          <DropdownMenuLabel>{moveLabel(publishing)}</DropdownMenuLabel>
          <DropdownMenuRadioGroup value={current ?? ""} onValueChange={moveTo}>
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
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
