import {
  useEffect,
  useId,
  useRef,
  useState,
  type FormEvent,
  type RefObject,
} from "react"
import { useQuery } from "@tanstack/react-query"
import { LoaderCircleIcon } from "lucide-react"
import { toast } from "sonner"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group"
import { Skeleton } from "@/components/ui/skeleton"
import { Textarea } from "@/components/ui/textarea"
import { SidePanel } from "@/shell/side-panel"

import type { PromptPanelTarget } from "./prompt-panel-store"
import {
  promptProblem,
  promptsQuery,
  useCreatePrompt,
  useUpdatePrompt,
  type PromptProblem,
  type SavedPrompt,
} from "./queries"
import {
  normalizePromptName,
  promptBodyError,
  promptNameError,
  suggestPromptName,
} from "./rules"

type Props = {
  target: PromptPanelTarget
  onClose: () => void
}

type Scope = "private" | "shared"

const panelDescription = "Run it again from the chat home."

function FieldError({ id, text }: { id: string; text: string | null }) {
  if (text === null) return null
  return (
    <p id={id} role="alert" className="text-sm text-destructive">
      {text}
    </p>
  )
}

function ObserveRun() {
  return <code className="font-mono text-xs">observe run</code>
}

function Spinner() {
  return (
    <LoaderCircleIcon
      aria-hidden="true"
      className="size-3.5 motion-safe:animate-spin"
    />
  )
}

export function PromptFormPanel({ target, onClose }: Props) {
  const [trigger] = useState(() => ({
    current: document.getElementById(target.returnFocusId),
  }))
  if (target.kind === "create") {
    return (
      <PromptForm
        key="create"
        target={target}
        prompt={null}
        trigger={trigger}
        onClose={onClose}
      />
    )
  }
  return (
    <EditPromptPanel
      promptId={target.promptId}
      trigger={trigger}
      target={target}
      onClose={onClose}
    />
  )
}

function EditPromptPanel({
  promptId,
  target,
  trigger,
  onClose,
}: Props & { promptId: string; trigger: RefObject<HTMLElement | null> }) {
  const query = useQuery(promptsQuery())
  const prompt = query.data?.find((item) => item.id === promptId)

  if (prompt !== undefined) {
    return (
      <PromptForm
        key={prompt.id}
        target={target}
        prompt={prompt}
        trigger={trigger}
        onClose={onClose}
      />
    )
  }
  if (query.isPending) {
    return (
      <SidePanel
        title="Edit prompt"
        description={panelDescription}
        trigger={trigger}
        onClose={onClose}
        footer={
          <>
            <Button variant="outline" onClick={onClose}>
              Cancel
            </Button>
            <Button type="submit" focusableWhenDisabled disabled>
              Save changes
            </Button>
          </>
        }
      >
        <div aria-busy="true" className="flex flex-col gap-4">
          <Skeleton className="h-8 w-full" />
          <Skeleton className="h-28 w-full" />
        </div>
      </SidePanel>
    )
  }
  const text = query.isError
    ? promptProblem(query.error, { name: "", shared: false }).text
    : "This prompt no longer exists."
  return (
    <SidePanel
      title="Edit prompt"
      description={panelDescription}
      trigger={trigger}
      onClose={onClose}
      footer={
        <Button variant="outline" onClick={onClose}>
          Close
        </Button>
      }
    >
      <Alert variant="destructive">
        <AlertDescription>{text}</AlertDescription>
      </Alert>
    </SidePanel>
  )
}

function PromptForm({
  target,
  prompt,
  trigger,
  onClose,
}: Props & {
  prompt: SavedPrompt | null
  trigger: RefObject<HTMLElement | null>
}) {
  const ids = useId()
  const nameInput = useRef<HTMLInputElement>(null)
  const bodyInput = useRef<HTMLTextAreaElement>(null)
  const alertBox = useRef<HTMLDivElement>(null)
  const createPrompt = useCreatePrompt()
  const updatePrompt = useUpdatePrompt()

  const creating = prompt === null
  const [name, setName] = useState(
    prompt?.name ??
      (target.kind === "create" ? suggestPromptName(target.body) : "")
  )
  const [body, setBody] = useState(
    prompt?.body ?? (target.kind === "create" ? target.body : "")
  )
  const [scope, setScope] = useState<Scope>(
    prompt?.shared ? "shared" : "private"
  )
  const [submitted, setSubmitted] = useState(false)
  const [problem, setProblem] = useState<PromptProblem | null>(null)
  const pending = createPrompt.isPending || updatePrompt.isPending

  useEffect(() => {
    nameInput.current?.select()
  }, [])

  useEffect(() => {
    if (problem === null) return
    const field =
      problem.field === "name"
        ? nameInput.current
        : problem.field === "body"
          ? bodyInput.current
          : alertBox.current
    field?.focus()
  }, [problem])

  const nameProblem = promptNameError(name)
  const bodyProblem = promptBodyError(body)
  const nameText =
    (submitted || name.length > 0 ? nameProblem : null) ??
    (problem?.field === "name" ? problem.text : null)
  const bodyText =
    (submitted || body.length > 0 ? bodyProblem : null) ??
    (problem?.field === "body" ? problem.text : null)

  function submit(event: FormEvent) {
    event.preventDefault()
    setSubmitted(true)
    if (pending) return
    if (nameProblem !== null) {
      nameInput.current?.focus()
      return
    }
    if (bodyProblem !== null) {
      bodyInput.current?.focus()
      return
    }
    const finalName = normalizePromptName(name)
    const finalBody = body.trim()
    const shared = scope === "shared"
    const attempt = { name: finalName, shared }
    setProblem(null)
    const callbacks = {
      onSuccess: () => {
        toast.success(
          creating ? `Saved "${finalName}"` : `Updated "${finalName}"`
        )
        onClose()
      },
      onError: (error: unknown) => setProblem(promptProblem(error, attempt)),
    }
    if (prompt === null) {
      createPrompt.mutate(
        { name: finalName, body: finalBody, shared },
        callbacks
      )
      return
    }
    const patch = {
      ...(finalName !== prompt.name ? { name: finalName } : {}),
      ...(finalBody !== prompt.body ? { body: finalBody } : {}),
      ...(shared !== prompt.shared ? { shared } : {}),
    }
    if (Object.keys(patch).length === 0) {
      onClose()
      return
    }
    updatePrompt.mutate({ id: prompt.id, patch }, callbacks)
  }

  const formId = `${ids}-form`
  const nameId = `${ids}-name`
  const nameHintId = `${ids}-name-hint`
  const nameErrorId = `${ids}-name-error`
  const bodyId = `${ids}-body`
  const bodyHintId = `${ids}-body-hint`
  const bodyErrorId = `${ids}-body-error`
  const sharedHintId = `${ids}-shared-hint`

  function describedBy(
    hintId: string | null,
    errorId: string,
    text: string | null
  ) {
    return (
      [hintId, text === null ? null : errorId].filter(Boolean).join(" ") ||
      undefined
    )
  }

  const bodyHint = creating
    ? "Copied from the chat you saved it from. Edit it freely."
    : null

  return (
    <SidePanel
      title={creating ? "Save prompt" : "Edit prompt"}
      description={panelDescription}
      initialFocus={nameInput}
      trigger={trigger}
      closeDisabled={pending}
      onClose={onClose}
      footer={
        <>
          <Button variant="outline" disabled={pending} onClick={onClose}>
            Cancel
          </Button>
          <Button
            type="submit"
            form={formId}
            focusableWhenDisabled
            disabled={pending}
            aria-busy={pending}
          >
            {pending ? <Spinner /> : null}
            {creating ? "Save prompt" : "Save changes"}
          </Button>
        </>
      }
    >
      {problem !== null && problem.field === null ? (
        <Alert ref={alertBox} variant="destructive" tabIndex={-1}>
          <AlertDescription>{problem.text}</AlertDescription>
        </Alert>
      ) : null}
      <form
        id={formId}
        noValidate
        onSubmit={submit}
        className="flex flex-col gap-4"
      >
        <div className="flex flex-col gap-1.5">
          <Label htmlFor={nameId}>Name</Label>
          <Input
            id={nameId}
            ref={nameInput}
            value={name}
            autoComplete="off"
            spellCheck={false}
            readOnly={pending}
            aria-invalid={nameText !== null}
            aria-describedby={describedBy(nameHintId, nameErrorId, nameText)}
            onChange={(event) => {
              setName(event.target.value)
              setProblem((current) =>
                current?.field === "name" ? null : current
              )
            }}
          />
          <p id={nameHintId} className="text-xs text-muted-foreground">
            Shown in the picker and used by <ObserveRun />. Letters, numbers,
            spaces, - and _, up to 64 characters.
          </p>
          <FieldError id={nameErrorId} text={nameText} />
        </div>
        <div className="flex flex-col gap-1.5">
          <Label htmlFor={bodyId}>Prompt</Label>
          <Textarea
            id={bodyId}
            ref={bodyInput}
            rows={5}
            className="resize-y"
            value={body}
            readOnly={pending}
            aria-invalid={bodyText !== null}
            aria-describedby={describedBy(
              bodyHint === null ? null : bodyHintId,
              bodyErrorId,
              bodyText
            )}
            onChange={(event) => {
              setBody(event.target.value)
              setProblem((current) =>
                current?.field === "body" ? null : current
              )
            }}
          />
          {bodyHint === null ? null : (
            <p id={bodyHintId} className="text-xs text-muted-foreground">
              {bodyHint}
            </p>
          )}
          <FieldError id={bodyErrorId} text={bodyText} />
        </div>
        <fieldset className="flex flex-col gap-2">
          <legend className="mb-2 text-[13px] font-medium">
            Who can use it
          </legend>
          <RadioGroup
            value={scope}
            disabled={pending}
            onValueChange={(next) => setScope(next as Scope)}
          >
            <label className="flex items-center gap-2 text-[13px]">
              <RadioGroupItem value="private" />
              Only me
            </label>
            <div className="flex flex-col gap-0.5">
              <label className="flex items-center gap-2 text-[13px]">
                <RadioGroupItem
                  value="shared"
                  aria-describedby={sharedHintId}
                />
                Everyone on this server
              </label>
              <p
                id={sharedHintId}
                className="pl-6 text-xs text-muted-foreground"
              >
                They can run it here and with <ObserveRun />. Only you and
                admins can edit it.
              </p>
            </div>
          </RadioGroup>
        </fieldset>
      </form>
    </SidePanel>
  )
}
