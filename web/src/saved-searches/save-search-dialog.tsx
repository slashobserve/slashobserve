import { useId, useRef, useState, type FormEvent } from "react"
import { useQuery } from "@tanstack/react-query"
import { LoaderCircleIcon } from "lucide-react"
import { toast } from "sonner"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { foldersQuery } from "@/folders/queries"
import { loadProblemText } from "@/service-accounts/queries"

import { defaultTitle, saveTargets, type SearchDraft } from "./draft"
import { useSaveSearch } from "./queries"

const maxTitleLength = 120

type Props = {
  draft: SearchDraft
  open: boolean
  onOpenChange: (open: boolean) => void
  finalFocus: () => HTMLElement | boolean
}

export function SaveSearchDialog({
  draft,
  open,
  onOpenChange,
  finalFocus,
}: Props) {
  const ids = useId()
  const titleRef = useRef<HTMLInputElement>(null)
  const folders = useQuery(foldersQuery())
  const save = useSaveSearch()
  const [titleEdit, setTitleEdit] = useState<string>()
  const [folderEdit, setFolderEdit] = useState<string>()
  const [problem, setProblem] = useState<string | null>(null)

  const targets = saveTargets(folders.data ?? [])
  const title = titleEdit ?? defaultTitle(draft)
  const folder = folderEdit ?? targets.initial
  const items = targets.choices.map((choice) => ({
    value: choice.id,
    label: choice.title,
  }))
  const noun = draft.kind === "logs" ? "logs search" : "trace search"

  function close() {
    onOpenChange(false)
    setTitleEdit(undefined)
    setFolderEdit(undefined)
    setProblem(null)
    save.reset()
  }

  function changeOpen(next: boolean) {
    if (next) onOpenChange(true)
    else if (!save.isPending) close()
  }

  function submit(event: FormEvent) {
    event.preventDefault()
    if (title.trim() === "" || folder === null || save.isPending) return
    setProblem(null)
    const folderTitle = items.find((item) => item.value === folder)?.label
    save.mutate(
      { ...draft, title: title.trim(), folder_id: folder },
      {
        onSuccess: (saved) => {
          toast.success(`Saved "${saved.title}" in ${folderTitle}`)
          close()
        },
        onError: (error) => setProblem(loadProblemText(error)),
      }
    )
  }

  return (
    <Dialog open={open} onOpenChange={changeOpen}>
      <DialogContent
        className="sm:max-w-md"
        initialFocus={titleRef}
        finalFocus={finalFocus}
      >
        <form className="grid gap-4" onSubmit={submit} noValidate>
          <DialogHeader>
            <DialogTitle>Save search</DialogTitle>
            <DialogDescription>
              {`Saves this ${noun} with its time range. Everyone who can view the folder can open it.`}
            </DialogDescription>
          </DialogHeader>
          <div className="grid gap-2">
            <Label htmlFor={`${ids}-title`}>Title</Label>
            <Input
              id={`${ids}-title`}
              ref={titleRef}
              autoComplete="off"
              maxLength={maxTitleLength}
              value={title}
              readOnly={save.isPending}
              onChange={(event) => setTitleEdit(event.target.value)}
            />
          </div>
          <div className="grid gap-2">
            <Label htmlFor={`${ids}-folder`}>Folder</Label>
            <Select
              items={items}
              value={folder}
              disabled={save.isPending}
              onValueChange={(next) => setFolderEdit(next ?? undefined)}
            >
              <SelectTrigger id={`${ids}-folder`} className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent alignItemWithTrigger={false} align="start">
                {items.map((item) => (
                  <SelectItem key={item.value} value={item.value}>
                    {item.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          {problem !== null ? (
            <Alert variant="destructive">
              <AlertDescription>{problem}</AlertDescription>
            </Alert>
          ) : null}
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              aria-disabled={save.isPending}
              onClick={() => changeOpen(false)}
            >
              Cancel
            </Button>
            <Button
              type="submit"
              focusableWhenDisabled
              disabled={
                save.isPending || title.trim() === "" || folder === null
              }
              aria-busy={save.isPending}
            >
              {save.isPending ? (
                <LoaderCircleIcon
                  aria-hidden="true"
                  className="motion-safe:animate-spin"
                />
              ) : null}
              Save
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
