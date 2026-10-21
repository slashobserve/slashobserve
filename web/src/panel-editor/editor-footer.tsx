import { useId } from "react"
import { LoaderCircleIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"

export const maxNoteLength = 500

type Props = {
  summary: string
  noteOpen: boolean
  note: string
  onNoteOpen: () => void
  onNoteChange: (note: string) => void
  saving: boolean
  canSave: boolean
  saveLabel: string
  onSave: () => void
  onDiscard: () => void
  readOnly: boolean
  onOpenLatest: () => void
  status: string | null
}

export function EditorFooter({
  summary,
  noteOpen,
  note,
  onNoteOpen,
  onNoteChange,
  saving,
  canSave,
  saveLabel,
  onSave,
  onDiscard,
  readOnly,
  onOpenLatest,
  status,
}: Props) {
  const noteId = useId()
  if (readOnly) {
    return (
      <div className="flex flex-wrap items-center justify-end gap-2 border-t px-4 pt-3 pb-3.5">
        <span className="mr-auto text-xs text-muted-foreground">
          Open the latest version to edit this panel.
        </span>
        <Button variant="outline" onClick={onOpenLatest}>
          Open latest version
        </Button>
      </div>
    )
  }
  return (
    <div className="flex flex-wrap items-center justify-end gap-2 border-t px-4 pt-3 pb-3.5">
      {noteOpen ? (
        <div className="flex w-full flex-col gap-1.5">
          <Label htmlFor={noteId} className="text-[13px]">
            Note for the timeline
          </Label>
          <Input
            id={noteId}
            autoFocus
            value={note}
            placeholder="Optional"
            maxLength={maxNoteLength}
            onChange={(event) => onNoteChange(event.target.value)}
          />
        </div>
      ) : null}
      {status === null ? null : (
        <p
          role="status"
          className="flex w-full items-center gap-2 text-xs text-muted-foreground"
        >
          <LoaderCircleIcon
            aria-hidden="true"
            className="size-3.5 motion-safe:animate-spin"
          />
          {status}
        </p>
      )}
      <span className="mr-auto text-xs text-muted-foreground">{summary}</span>
      {noteOpen ? null : (
        <Button
          variant="ghost"
          className="text-muted-foreground"
          disabled={saving}
          onClick={onNoteOpen}
        >
          Add a note
        </Button>
      )}
      <Button variant="outline" disabled={saving} onClick={onDiscard}>
        Discard
      </Button>
      <Button disabled={!canSave} onClick={onSave}>
        {saveLabel}
      </Button>
    </div>
  )
}
