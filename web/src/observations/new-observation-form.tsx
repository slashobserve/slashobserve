import { useEffect, useState, type FormEvent, type RefObject } from "react"
import { useNavigate } from "@tanstack/react-router"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { observationLink } from "@/lib/links"
import { loadProblemText } from "@/service-accounts/queries"

import { FolderSelect } from "./folder-select"
import type { useCreateObservation } from "./queries"

type Props = {
  create: ReturnType<typeof useCreateObservation>
  formId: string
  titleRef: RefObject<HTMLInputElement | null>
  autoFocus?: boolean
}

const maxTitleLength = 200
const titleProblem = "Enter a title of 1 to 200 characters."

function titleError(title: string): string | null {
  const length = title.trim().length
  return length >= 1 && length <= maxTitleLength ? null : titleProblem
}

export function NewObservationForm({
  create,
  formId,
  titleRef,
  autoFocus = false,
}: Props) {
  const navigate = useNavigate()
  const [title, setTitle] = useState("")
  const [folderId, setFolderId] = useState<string | null>(null)
  const [submitted, setSubmitted] = useState(false)
  const [problem, setProblem] = useState<string | null>(null)

  useEffect(() => {
    if (autoFocus) titleRef.current?.focus()
  }, [autoFocus, titleRef])

  const invalid = titleError(title)
  const showError = invalid !== null && (submitted || title.length > 0)
  const errorId = `${formId}-title-error`

  function submit(event: FormEvent) {
    event.preventDefault()
    setSubmitted(true)
    if (create.isPending) return
    if (invalid !== null) {
      titleRef.current?.focus()
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
    <form
      id={formId}
      noValidate
      onSubmit={submit}
      className="flex flex-col gap-4"
    >
      <div className="flex flex-col gap-1.5">
        <Label htmlFor={`${formId}-title`}>Title</Label>
        <Input
          id={`${formId}-title`}
          ref={titleRef}
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
        <Label htmlFor={`${formId}-folder`}>Folder</Label>
        <FolderSelect
          id={`${formId}-folder`}
          value={folderId}
          disabled={create.isPending}
          onChange={setFolderId}
        />
      </div>
      {problem !== null ? (
        <Alert variant="destructive">
          <AlertDescription>{problem}</AlertDescription>
        </Alert>
      ) : null}
    </form>
  )
}
