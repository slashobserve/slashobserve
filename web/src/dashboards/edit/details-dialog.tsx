import {
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  type FormEvent,
  type ReactNode,
} from "react"
import { useQueryClient } from "@tanstack/react-query"
import { LoaderCircleIcon } from "lucide-react"
import { toast } from "sonner"

import { loadProblemText } from "@/api/errors"
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
import { Textarea } from "@/components/ui/textarea"
import { moveChoices } from "@/folders/move-targets"
import type { Folder } from "@/folders/queries"
import { cleanText } from "@/lib/user-text"
import { dashboardKeys } from "@/api/keys"
import { panelTitles } from "@/history/diff-groups"
import { loadSpecValidator, type Dashboard } from "@/spec"

import { useAddVersion, useMoveDashboard } from "../queries"
import { saveToast } from "../save-outcome"
import {
  applyDetails,
  descriptionError,
  detailsOf,
  detailsSummary,
  parseTags,
  sameDetails,
  titleError,
  type Details,
} from "./details"
import { MergeConflicts } from "./merge-conflicts"
import { saveProblemFrom, type Problem } from "./save-problem"

const rejectedHeading = "The server rejected these details:"
const invalidHeading = "These details are not a valid Spec:"

type Edits = {
  title?: string
  description?: string
  tags?: string
  folder?: string
}

type Props = {
  dashboardId: string
  spec: Dashboard
  version: number
  folder: { id: string; personal: boolean }
  folders: readonly Folder[]
  open: boolean
  onOpenChange: (open: boolean) => void
  finalFocus: () => HTMLElement | boolean
}

export function DetailsDialog({
  dashboardId,
  spec,
  version,
  folder,
  folders,
  open,
  onOpenChange,
  finalFocus,
}: Props) {
  const queryClient = useQueryClient()
  const save = useAddVersion(dashboardId)
  const move = useMoveDashboard()
  const ids = useId()
  const titleId = `${ids}-title`
  const descriptionId = `${ids}-description`
  const tagsId = `${ids}-tags`
  const folderId = `${ids}-folder`
  const titleRef = useRef<HTMLInputElement>(null)
  const reloadRef = useRef<HTMLButtonElement>(null)
  const [edits, setEdits] = useState<Edits>({})
  const [submitted, setSubmitted] = useState(false)
  const [problem, setProblem] = useState<Problem | null>(null)
  const [preparing, setPreparing] = useState(false)
  const pending = save.isPending || preparing || move.isPending
  const titles = useMemo(() => panelTitles(spec), [spec])

  const current = detailsOf(spec)
  const title = edits.title ?? current.title
  const description = edits.description ?? current.description
  const tagsText = edits.tags ?? current.tags.join(", ")
  const titleProblem = titleError(title)
  const descriptionProblem = descriptionError(description)
  const parsedTags = parseTags(tagsText)
  const tagsProblem = "error" in parsedTags ? parsedTags.error : null
  const next: Details | null =
    titleProblem === null && descriptionProblem === null && !tagsProblem
      ? {
          title: cleanText(title),
          description: description.trim(),
          tags: "tags" in parsedTags ? parsedTags.tags : [],
        }
      : null
  const folderItems = moveChoices(folders, folder.id).map((choice) => ({
    value: choice.id,
    label: choice.title,
  }))
  const folderChoice = edits.folder ?? folder.id
  const folderChanged = folderChoice !== folder.id
  const detailsChanged = next !== null && !sameDetails(current, next)
  const unchanged = next !== null && !detailsChanged && !folderChanged
  const shown = (field: keyof Edits, message: string | null) =>
    message !== null && (submitted || edits[field] !== undefined)
      ? message
      : null

  const conflicted = problem?.kind === "conflict"
  useEffect(() => {
    if (conflicted) reloadRef.current?.focus()
  }, [conflicted])

  function close() {
    onOpenChange(false)
    setEdits({})
    setSubmitted(false)
    setProblem(null)
    save.reset()
    move.reset()
  }

  function changeOpen(value: boolean) {
    if (value) onOpenChange(true)
    else if (!pending) close()
  }

  function moveToChosen() {
    const target = folderItems.find((item) => item.value === folderChoice)
    const verb = folder.personal ? "Published" : "Moved"
    move.mutate(
      { id: dashboardId, folderId: folderChoice },
      {
        onSuccess: () => {
          toast.success(`${verb} to ${target?.label ?? "the folder"}`)
          close()
        },
        onError: (failure) =>
          setProblem({
            kind: "failed",
            message: `Could not move the dashboard. ${loadProblemText(failure)}`,
          }),
      }
    )
  }

  async function send(details: Details, base: Dashboard, parent: number) {
    setPreparing(true)
    let validate
    try {
      validate = await loadSpecValidator()
    } catch (failure) {
      setProblem(saveProblemFrom(failure, rejectedHeading))
      return
    } finally {
      setPreparing(false)
    }
    const checked = validate(applyDetails(base, details))
    if (!checked.valid) {
      setProblem({
        kind: "invalid",
        heading: invalidHeading,
        lines: [
          ...new Set(
            checked.issues.map((issue) => `${issue.path}: ${issue.message}`)
          ),
        ],
      })
      return
    }
    save.mutate(
      {
        spec: checked.spec,
        summary: detailsSummary(detailsOf(base), details),
        parent_version: parent,
        propose: false,
      },
      {
        onSuccess: (outcome) => {
          toast.success(saveToast(outcome))
          if (folderChanged) moveToChosen()
          else close()
        },
        onError: (failure) =>
          setProblem(saveProblemFrom(failure, rejectedHeading)),
      }
    )
  }

  function submit(event: FormEvent) {
    event.preventDefault()
    setSubmitted(true)
    if (next === null || unchanged || pending) return
    setProblem(null)
    if (detailsChanged) void send(next, spec, version)
    else moveToChosen()
  }

  function reload() {
    void queryClient.invalidateQueries({
      queryKey: dashboardKeys.latest(dashboardId),
    })
    close()
  }

  const spinner = pending ? (
    <LoaderCircleIcon
      aria-hidden="true"
      className="size-3.5 motion-safe:animate-spin"
    />
  ) : null

  return (
    <Dialog open={open} onOpenChange={changeOpen}>
      <DialogContent
        className="sm:max-w-md"
        initialFocus={titleRef}
        finalFocus={finalFocus}
      >
        {problem?.kind === "conflict" ? (
          <>
            <DialogHeader>
              <DialogTitle>Edit details</DialogTitle>
              <DialogDescription>
                Your details were not saved.
              </DialogDescription>
            </DialogHeader>
            <MergeConflicts
              conflicts={problem.conflicts}
              titles={titles}
              latest={problem.latest}
              onReload={reload}
              reloadRef={reloadRef}
            />
            <DialogFooter>
              <Button variant="outline" onClick={() => changeOpen(false)}>
                Cancel
              </Button>
            </DialogFooter>
          </>
        ) : (
          <form className="grid gap-4" onSubmit={submit} noValidate>
            <DialogHeader>
              <DialogTitle>Edit details</DialogTitle>
              <DialogDescription>
                Saves a new version with this title, description and tags, and
                moves the dashboard when you choose another folder.
              </DialogDescription>
            </DialogHeader>
            <Field
              id={titleId}
              label="Title"
              error={shown("title", titleProblem)}
            >
              <Input
                id={titleId}
                ref={titleRef}
                autoComplete="off"
                value={title}
                readOnly={pending}
                aria-invalid={shown("title", titleProblem) !== null}
                aria-describedby={
                  shown("title", titleProblem) !== null
                    ? `${titleId}-error`
                    : undefined
                }
                onChange={(event) =>
                  setEdits({ ...edits, title: event.target.value })
                }
              />
            </Field>
            <Field
              id={descriptionId}
              label="Description"
              error={shown("description", descriptionProblem)}
            >
              <Textarea
                id={descriptionId}
                rows={4}
                value={description}
                readOnly={pending}
                aria-invalid={shown("description", descriptionProblem) !== null}
                aria-describedby={
                  shown("description", descriptionProblem) !== null
                    ? `${descriptionId}-error`
                    : undefined
                }
                onChange={(event) =>
                  setEdits({ ...edits, description: event.target.value })
                }
              />
            </Field>
            <Field
              id={tagsId}
              label="Tags"
              error={shown("tags", tagsProblem)}
              hint="Separate tags with commas or spaces."
            >
              <Input
                id={tagsId}
                autoComplete="off"
                value={tagsText}
                readOnly={pending}
                aria-invalid={shown("tags", tagsProblem) !== null}
                aria-describedby={
                  shown("tags", tagsProblem) !== null
                    ? `${tagsId}-error`
                    : `${tagsId}-hint`
                }
                onChange={(event) =>
                  setEdits({ ...edits, tags: event.target.value })
                }
              />
            </Field>
            <Field
              id={folderId}
              label="Folder"
              error={null}
              hint={
                folderItems.length > 1
                  ? undefined
                  : "There is no other folder you can move it to."
              }
            >
              <Select
                items={folderItems}
                value={folderChoice}
                disabled={pending || folderItems.length < 2}
                onValueChange={(value) =>
                  setEdits({ ...edits, folder: value ?? undefined })
                }
              >
                <SelectTrigger id={folderId} className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent alignItemWithTrigger={false} align="start">
                  {folderItems.map((item) => (
                    <SelectItem key={item.value} value={item.value}>
                      {item.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
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
            {save.isPending || preparing ? (
              <p className="text-sm text-muted-foreground">
                Checking every panel query before saving.
              </p>
            ) : null}
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                aria-disabled={pending}
                onClick={() => changeOpen(false)}
              >
                Cancel
              </Button>
              <Button
                type="submit"
                focusableWhenDisabled
                disabled={pending || unchanged}
                aria-busy={pending}
              >
                {spinner}
                {pending
                  ? "Saving"
                  : problem?.kind === "failed"
                    ? "Try again"
                    : "Save"}
              </Button>
            </DialogFooter>
          </form>
        )}
      </DialogContent>
    </Dialog>
  )
}

function Field({
  id,
  label,
  error,
  hint,
  children,
}: {
  id: string
  label: string
  error: string | null
  hint?: string
  children: ReactNode
}) {
  return (
    <div className="grid gap-2">
      <Label htmlFor={id}>{label}</Label>
      {children}
      {error !== null ? (
        <p id={`${id}-error`} role="alert" className="text-sm text-destructive">
          {error}
        </p>
      ) : hint !== undefined ? (
        <p id={`${id}-hint`} className="text-sm text-muted-foreground">
          {hint}
        </p>
      ) : null}
    </div>
  )
}
