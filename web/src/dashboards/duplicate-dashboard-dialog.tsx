import { useId, useRef, useState, type FormEvent } from "react"
import { useNavigate } from "@tanstack/react-router"
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

import { titleError } from "./edit/details"
import { saveProblemFrom, type Problem } from "./edit/save-problem"
import { duplicateTitle, type DuplicateTargets } from "./duplicate"
import { useDuplicateDashboard } from "./queries"

const rejectedHeading = "The server rejected this copy:"

type Props = {
  source: { id: string; title: string; version: number }
  targets: DuplicateTargets
  open: boolean
  onOpenChange: (open: boolean) => void
  finalFocus: () => HTMLElement | boolean
}

export function DuplicateDashboardDialog({
  source,
  targets,
  open,
  onOpenChange,
  finalFocus,
}: Props) {
  const navigate = useNavigate()
  const duplicate = useDuplicateDashboard(source.id)
  const ids = useId()
  const titleId = `${ids}-title`
  const folderId = `${ids}-folder`
  const titleRef = useRef<HTMLInputElement>(null)
  const openedCopy = useRef(false)
  const [titleEdit, setTitleEdit] = useState<string>()
  const [folderEdit, setFolderEdit] = useState<string>()
  const [submitted, setSubmitted] = useState(false)
  const [problem, setProblem] = useState<Problem | null>(null)

  const title = titleEdit ?? duplicateTitle(source.title)
  const folder = folderEdit ?? targets.initial
  const titleProblem = titleError(title)
  const shownTitleProblem =
    titleProblem !== null && (submitted || titleEdit !== undefined)
      ? titleProblem
      : null
  const items = targets.choices.map((choice) => ({
    value: choice.id,
    label: choice.title,
  }))

  function close() {
    onOpenChange(false)
    setTitleEdit(undefined)
    setFolderEdit(undefined)
    setSubmitted(false)
    setProblem(null)
    duplicate.reset()
  }

  function changeOpen(next: boolean) {
    if (next) onOpenChange(true)
    else if (!duplicate.isPending) close()
  }

  function submit(event: FormEvent) {
    event.preventDefault()
    setSubmitted(true)
    if (titleProblem !== null || folder === null || duplicate.isPending) return
    setProblem(null)
    const trimmed = title.trim()
    const folderTitle = items.find((item) => item.value === folder)?.label
    duplicate.mutate(
      { title: trimmed, folderId: folder },
      {
        onSuccess: (created) => {
          openedCopy.current = true
          toast.success(`Created "${trimmed}" in ${folderTitle}`)
          close()
          void navigate({
            to: "/dashboards/$dashboardId",
            params: { dashboardId: created.dashboard.id },
          })
        },
        onError: (failure) =>
          setProblem(saveProblemFrom(failure, rejectedHeading)),
      }
    )
  }

  return (
    <Dialog open={open} onOpenChange={changeOpen}>
      <DialogContent
        className="sm:max-w-md"
        initialFocus={titleRef}
        finalFocus={() => {
          const leaveFocusToCopy = openedCopy.current
          openedCopy.current = false
          return leaveFocusToCopy ? false : finalFocus()
        }}
      >
        <form className="grid gap-4" onSubmit={submit} noValidate>
          <DialogHeader>
            <DialogTitle>Duplicate dashboard</DialogTitle>
            <DialogDescription className="wrap-anywhere">
              {`Saves version ${source.version} of "${source.title}" as a new dashboard. Its history, comments and import record stay with the original.`}
            </DialogDescription>
          </DialogHeader>
          <div className="grid gap-2">
            <Label htmlFor={titleId}>Title</Label>
            <Input
              id={titleId}
              ref={titleRef}
              autoComplete="off"
              value={title}
              readOnly={duplicate.isPending}
              aria-invalid={shownTitleProblem !== null}
              aria-describedby={
                shownTitleProblem !== null ? `${titleId}-error` : undefined
              }
              onChange={(event) => setTitleEdit(event.target.value)}
            />
            {shownTitleProblem !== null ? (
              <p
                id={`${titleId}-error`}
                role="alert"
                className="text-sm text-destructive"
              >
                {shownTitleProblem}
              </p>
            ) : null}
          </div>
          <div className="grid gap-2">
            <Label htmlFor={folderId}>Folder</Label>
            <Select
              items={items}
              value={folder}
              disabled={duplicate.isPending}
              onValueChange={(next) => setFolderEdit(next ?? undefined)}
            >
              <SelectTrigger id={folderId} className="w-full">
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
          {problem?.kind === "invalid" ? (
            <Alert variant="destructive">
              <AlertDescription>
                <p>{problem.heading}</p>
                <ul className="list-disc pl-4">
                  {problem.lines.map((line) => (
                    <li key={line}>{line}</li>
                  ))}
                </ul>
              </AlertDescription>
            </Alert>
          ) : null}
          {problem?.kind === "failed" ? (
            <Alert variant="destructive">
              <AlertDescription>{problem.message}</AlertDescription>
            </Alert>
          ) : null}
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              aria-disabled={duplicate.isPending}
              onClick={() => changeOpen(false)}
            >
              Cancel
            </Button>
            <Button
              type="submit"
              focusableWhenDisabled
              disabled={
                duplicate.isPending || titleProblem !== null || folder === null
              }
              aria-busy={duplicate.isPending}
            >
              {duplicate.isPending ? (
                <LoaderCircleIcon
                  aria-hidden="true"
                  className="motion-safe:animate-spin"
                />
              ) : null}
              Duplicate
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
