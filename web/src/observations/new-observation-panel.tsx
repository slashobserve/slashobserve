import { useId, useRef, useState, type FormEvent, type RefObject } from "react"
import { useQuery } from "@tanstack/react-query"
import { useNavigate } from "@tanstack/react-router"
import { LoaderCircleIcon } from "lucide-react"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
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
import { observationLink } from "@/lib/links"
import { loadProblemText } from "@/service-accounts/queries"
import { SidePanel } from "@/shell/side-panel"

import { useCreateObservation } from "./queries"

type Props = {
  onClose: () => void
  trigger: RefObject<HTMLElement | null>
}

const maxTitleLength = 200
const titleProblem = "Enter a title of 1 to 200 characters."

function titleError(title: string): string | null {
  const length = title.trim().length
  return length >= 1 && length <= maxTitleLength ? null : titleProblem
}

export function NewObservationPanel({ onClose, trigger }: Props) {
  const ids = useId()
  const titleInput = useRef<HTMLInputElement>(null)
  const navigate = useNavigate()
  const folders = useQuery(foldersQuery())
  const create = useCreateObservation()
  const [title, setTitle] = useState("")
  const [folderId, setFolderId] = useState<string | null>(null)
  const [submitted, setSubmitted] = useState(false)
  const [problem, setProblem] = useState<string | null>(null)

  const folderItems = (folders.data ?? [])
    .filter((folder) => folder.permissions.edit)
    .toSorted((a, b) => Number(b.personal) - Number(a.personal))
    .map((folder) => ({
      value: folder.id,
      label: folder.personal ? "My Dashboards" : folder.title,
    }))
  const invalid = titleError(title)
  const showError = invalid !== null && (submitted || title.length > 0)
  const errorId = `${ids}-title-error`
  const formId = `${ids}-form`

  function submit(event: FormEvent) {
    event.preventDefault()
    setSubmitted(true)
    if (create.isPending) return
    if (invalid !== null) {
      titleInput.current?.focus()
      return
    }
    setProblem(null)
    create.mutate(
      {
        title: title.trim(),
        ...(folderId === null ? {} : { folder_id: folderId }),
      },
      {
        onSuccess: (envelope) =>
          void navigate(observationLink(envelope.observation.id)),
        onError: (error) => setProblem(loadProblemText(error)),
      }
    )
  }

  return (
    <SidePanel
      title="New observation"
      initialFocus={titleInput}
      trigger={trigger}
      closeDisabled={create.isPending}
      onClose={onClose}
      footer={
        <>
          <Button
            variant="outline"
            disabled={create.isPending}
            onClick={onClose}
          >
            Cancel
          </Button>
          <Button
            type="submit"
            form={formId}
            focusableWhenDisabled
            disabled={create.isPending}
            aria-busy={create.isPending}
          >
            {create.isPending ? (
              <LoaderCircleIcon
                aria-hidden="true"
                className="size-3.5 motion-safe:animate-spin"
              />
            ) : null}
            Create
          </Button>
        </>
      }
    >
      <form
        id={formId}
        noValidate
        onSubmit={submit}
        className="flex flex-col gap-4"
      >
        <div className="flex flex-col gap-1.5">
          <Label htmlFor={`${ids}-title`}>Title</Label>
          <Input
            id={`${ids}-title`}
            ref={titleInput}
            value={title}
            autoComplete="off"
            required
            readOnly={create.isPending}
            aria-invalid={showError}
            aria-describedby={showError ? errorId : undefined}
            onChange={(event) => setTitle(event.target.value)}
          />
          {showError ? (
            <p id={errorId} role="alert" className="text-sm text-destructive">
              {invalid}
            </p>
          ) : null}
        </div>
        <div className="flex flex-col gap-1.5">
          <Label htmlFor={`${ids}-folder`}>Folder</Label>
          <Select
            items={folderItems}
            value={folderId}
            disabled={create.isPending}
            onValueChange={setFolderId}
          >
            <SelectTrigger id={`${ids}-folder`} className="w-full">
              <SelectValue placeholder="Default folder" />
            </SelectTrigger>
            <SelectContent alignItemWithTrigger={false} align="start">
              {folderItems.map((item) => (
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
      </form>
    </SidePanel>
  )
}
