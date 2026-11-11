import { useId, useState, type FormEvent } from "react"
import { LoaderCircleIcon } from "lucide-react"
import { toast } from "sonner"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { loadProblemText } from "@/service-accounts/queries"

import { useRenameSavedSearch, type SavedSearch } from "./queries"

const maxTitleLength = 120

type Props = {
  saved: SavedSearch
  open: boolean
  onOpenChange: (open: boolean) => void
  finalFocus: () => HTMLElement | boolean
}

export function RenameSavedSearchDialog({
  saved,
  open,
  onOpenChange,
  finalFocus,
}: Props) {
  const titleId = useId()
  const rename = useRenameSavedSearch(saved.id)
  const [edit, setEdit] = useState<string>()
  const [problem, setProblem] = useState<string | null>(null)
  const title = edit ?? saved.title

  function close() {
    onOpenChange(false)
    setEdit(undefined)
    setProblem(null)
    rename.reset()
  }

  function changeOpen(next: boolean) {
    if (next) onOpenChange(true)
    else if (!rename.isPending) close()
  }

  function submit(event: FormEvent) {
    event.preventDefault()
    const trimmed = title.trim()
    if (trimmed === "" || rename.isPending) return
    if (trimmed === saved.title) {
      close()
      return
    }
    setProblem(null)
    rename.mutate(trimmed, {
      onSuccess: () => {
        toast.success(`Renamed to ${trimmed}`)
        close()
      },
      onError: (error) => setProblem(loadProblemText(error)),
    })
  }

  return (
    <Dialog open={open} onOpenChange={changeOpen}>
      <DialogContent className="sm:max-w-md" finalFocus={finalFocus}>
        <form className="grid gap-4" onSubmit={submit} noValidate>
          <DialogHeader>
            <DialogTitle>Rename saved search</DialogTitle>
          </DialogHeader>
          <div className="grid gap-2">
            <Label htmlFor={titleId}>Title</Label>
            <Input
              id={titleId}
              autoComplete="off"
              maxLength={maxTitleLength}
              value={title}
              readOnly={rename.isPending}
              onChange={(event) => setEdit(event.target.value)}
            />
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
              aria-disabled={rename.isPending}
              onClick={() => changeOpen(false)}
            >
              Cancel
            </Button>
            <Button
              type="submit"
              focusableWhenDisabled
              disabled={rename.isPending || title.trim() === ""}
              aria-busy={rename.isPending}
            >
              {rename.isPending ? (
                <LoaderCircleIcon
                  aria-hidden="true"
                  className="motion-safe:animate-spin"
                />
              ) : null}
              Rename
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
