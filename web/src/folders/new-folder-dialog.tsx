import { useId, useState, type FormEvent, type RefObject } from "react"
import { LoaderCircleIcon } from "lucide-react"

import { ApiError } from "@/api/errors"
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

import { useCreateFolder, type Folder } from "./queries"

const maxTitleLength = 100

function createProblemText(error: unknown): string {
  if (error instanceof ApiError && error.code === "folder_exists") {
    return "A folder with this name already exists."
  }
  return loadProblemText(error)
}

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
  onCreated: (folder: Folder) => void
  finalFocus?: RefObject<HTMLElement | null>
}

export function NewFolderDialog({
  open,
  onOpenChange,
  onCreated,
  finalFocus,
}: Props) {
  const inputId = useId()
  const create = useCreateFolder()
  const [title, setTitle] = useState("")
  const [problem, setProblem] = useState<string | null>(null)
  const trimmed = title.trim()

  function close() {
    onOpenChange(false)
    setTitle("")
    setProblem(null)
    create.reset()
  }

  function submit(event: FormEvent) {
    event.preventDefault()
    if (trimmed === "" || create.isPending) return
    setProblem(null)
    create.mutate(trimmed, {
      onSuccess: (folder) => {
        close()
        onCreated(folder)
      },
      onError: (error) => setProblem(createProblemText(error)),
    })
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (next) onOpenChange(true)
        else if (!create.isPending) close()
      }}
    >
      <DialogContent finalFocus={finalFocus}>
        <form onSubmit={submit} className="grid gap-4">
          <DialogHeader>
            <DialogTitle>New folder</DialogTitle>
          </DialogHeader>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor={inputId}>Name</Label>
            <Input
              id={inputId}
              maxLength={maxTitleLength}
              autoComplete="off"
              value={title}
              onChange={(event) => {
                setTitle(event.target.value)
                setProblem(null)
              }}
            />
          </div>
          {problem === null ? null : (
            <Alert variant="destructive">
              <AlertDescription>{problem}</AlertDescription>
            </Alert>
          )}
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              disabled={create.isPending}
              onClick={close}
            >
              Cancel
            </Button>
            <Button
              type="submit"
              disabled={trimmed === ""}
              aria-busy={create.isPending}
            >
              {create.isPending ? (
                <LoaderCircleIcon
                  aria-hidden="true"
                  className="motion-safe:animate-spin"
                />
              ) : null}
              Create folder
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
