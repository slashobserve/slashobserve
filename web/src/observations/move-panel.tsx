import { useId, useState, type RefObject } from "react"
import { useQuery } from "@tanstack/react-query"
import { toast } from "sonner"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import {
  Combobox,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxInput,
  ComboboxItem,
  ComboboxList,
} from "@/components/ui/combobox"
import { Label } from "@/components/ui/label"
import { moveChoices } from "@/folders/move-targets"
import { foldersQuery } from "@/folders/queries"
import { loadProblemText } from "@/service-accounts/queries"
import { SidePanel } from "@/shell/side-panel"

import { useMoveObservation, type Observation } from "./queries"

type Choice = { id: string; title: string }

type Props = {
  observation: Observation
  onClose: () => void
  trigger: RefObject<HTMLElement | null>
}

export function MovePanel({ observation, onClose, trigger }: Props) {
  const fieldId = useId()
  const folders = useQuery(foldersQuery())
  const move = useMoveObservation(observation.id)
  const [target, setTarget] = useState<Choice | null>(null)
  const [problem, setProblem] = useState<string | null>(null)
  const choices: Choice[] = moveChoices(
    folders.data ?? [],
    observation.folder_id
  )
    .filter((folder) => folder.id !== observation.folder_id)
    .map((folder) => ({ id: folder.id, title: folder.title }))

  function submit() {
    if (target === null || move.isPending) return
    setProblem(null)
    move.mutate(
      { folder_id: target.id },
      {
        onSuccess: () => {
          toast.success(`Moved to ${target.title}`)
          onClose()
        },
        onError: (error) => setProblem(loadProblemText(error)),
      }
    )
  }

  return (
    <SidePanel
      title="Move to folder"
      trigger={trigger}
      closeDisabled={move.isPending}
      onClose={onClose}
      footer={
        <>
          <Button variant="outline" disabled={move.isPending} onClick={onClose}>
            Cancel
          </Button>
          <Button
            focusableWhenDisabled
            disabled={target === null || move.isPending}
            aria-busy={move.isPending}
            onClick={submit}
          >
            Move
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-1.5">
        <Label htmlFor={fieldId}>Folder</Label>
        <Combobox<Choice>
          items={choices}
          value={target}
          disabled={move.isPending}
          itemToStringLabel={(item) => item.title}
          isItemEqualToValue={(item, value) => item.id === value.id}
          onValueChange={setTarget}
        >
          <ComboboxInput
            id={fieldId}
            disabled={move.isPending}
            placeholder="Search folders"
          />
          <ComboboxContent>
            <ComboboxEmpty>
              {folders.isPending ? "Loading folders" : "No folders found"}
            </ComboboxEmpty>
            <ComboboxList>
              {(item: Choice) => (
                <ComboboxItem key={item.id} value={item}>
                  {item.title}
                </ComboboxItem>
              )}
            </ComboboxList>
          </ComboboxContent>
        </Combobox>
      </div>
      {problem === null ? null : (
        <Alert variant="destructive">
          <AlertDescription>{problem}</AlertDescription>
        </Alert>
      )}
    </SidePanel>
  )
}
