import {
  useEffect,
  useId,
  useRef,
  useState,
  type FormEvent,
  type ReactNode,
  type RefObject,
} from "react"
import { Link } from "@tanstack/react-router"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"

import {
  audienceLabels,
  contextProblemText,
  isDomainNotAllowed,
  sourceProblems,
  trimSource,
  type Audience,
  type SourceInput,
  type SourceProblems,
} from "./problems"
import type { ContextSource } from "./queries"

type Props = {
  formId: string
  mode: "add" | "edit"
  source?: ContextSource
  serverError: unknown
  onSubmit: (input: SourceInput) => void
  firstInputRef: RefObject<HTMLInputElement | null>
}

const audiences = (Object.keys(audienceLabels) as Audience[]).map((value) => ({
  value,
  label: audienceLabels[value],
}))

function inputOf(source: ContextSource | undefined): SourceInput {
  return {
    uid: source?.uid ?? "",
    name: source?.name ?? "",
    description: source?.description ?? "",
    url: source?.url ?? "",
    audience: source?.audience ?? "everyone",
    secretHeader: source?.secret_header ?? "",
    secret: "",
    clearSecret: false,
  }
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
  error?: string
  hint?: string
  children: ReactNode
}) {
  return (
    <div className="flex flex-col gap-1.5">
      <Label htmlFor={id} className="text-[13px] font-medium">
        {label}
      </Label>
      {children}
      {hint === undefined ? null : (
        <p className="text-xs text-muted-foreground">{hint}</p>
      )}
      {error === undefined ? null : (
        <p id={`${id}-error`} className="text-xs text-destructive">
          {error}
        </p>
      )}
    </div>
  )
}

function describedBy(id: string, error: string | undefined) {
  return error === undefined ? undefined : `${id}-error`
}

function ServerProblem({
  error,
  alertRef,
}: {
  error: unknown
  alertRef: RefObject<HTMLDivElement | null>
}) {
  return (
    <Alert ref={alertRef} variant="destructive" tabIndex={-1}>
      <AlertDescription>
        {contextProblemText(error)}
        {isDomainNotAllowed(error) ? (
          <>
            {" "}
            <Link
              to="/settings"
              search={(prev) => ({ ...prev, panel: undefined })}
              hash="outbound-access"
              className="underline underline-offset-3"
            >
              Outbound access
            </Link>
          </>
        ) : null}
      </AlertDescription>
    </Alert>
  )
}

export function SourceForm({
  formId,
  mode,
  source,
  serverError,
  onSubmit,
  firstInputRef,
}: Props) {
  const baseId = useId()
  const alertBox = useRef<HTMLDivElement>(null)
  const [values, setValues] = useState(() => inputOf(source))
  const [problems, setProblems] = useState<SourceProblems>({})
  const secretStored = source?.secret_set ?? false

  useEffect(() => {
    if (serverError !== null && serverError !== undefined) {
      alertBox.current?.focus()
    }
  }, [serverError])

  function change(next: Partial<SourceInput>) {
    setValues((current) => ({ ...current, ...next }))
  }

  function submit(event: FormEvent) {
    event.preventDefault()
    const found = sourceProblems(values, mode, secretStored)
    setProblems(found)
    if (Object.keys(found).length > 0) return
    onSubmit(trimSource(values))
  }

  const uidId = `${baseId}-uid`
  const nameId = `${baseId}-name`
  const descriptionId = `${baseId}-description`
  const urlId = `${baseId}-url`
  const audienceId = `${baseId}-audience`
  const headerId = `${baseId}-header`
  const secretId = `${baseId}-secret`
  const clearId = `${baseId}-clear`
  const secretHint = secretStored
    ? "A secret is stored and never shown again. Enter a new one to replace it."
    : "Stored sealed on the server and never shown again."

  return (
    <form
      id={formId}
      noValidate
      onSubmit={submit}
      className="flex flex-col gap-4"
    >
      {serverError === null || serverError === undefined ? null : (
        <ServerProblem error={serverError} alertRef={alertBox} />
      )}
      {mode === "add" ? (
        <Field
          id={uidId}
          label="Id"
          error={problems.uid}
          hint="Names the source in the CLI and the audit log. It cannot change."
        >
          <Input
            id={uidId}
            ref={firstInputRef}
            className="font-mono narrow:h-11"
            autoComplete="off"
            spellCheck={false}
            placeholder="docs"
            value={values.uid}
            aria-invalid={problems.uid !== undefined}
            aria-describedby={describedBy(uidId, problems.uid)}
            onChange={(event) => change({ uid: event.target.value })}
          />
        </Field>
      ) : null}
      <Field id={nameId} label="Name" error={problems.name}>
        <Input
          id={nameId}
          ref={mode === "edit" ? firstInputRef : undefined}
          className="narrow:h-11"
          autoComplete="off"
          placeholder="Company docs"
          value={values.name}
          aria-invalid={problems.name !== undefined}
          aria-describedby={describedBy(nameId, problems.name)}
          onChange={(event) => change({ name: event.target.value })}
        />
      </Field>
      <Field
        id={descriptionId}
        label="Description"
        error={problems.description}
        hint="What the source is for, in one line. Optional."
      >
        <Input
          id={descriptionId}
          className="narrow:h-11"
          autoComplete="off"
          placeholder="Runbooks and design notes"
          value={values.description}
          aria-invalid={problems.description !== undefined}
          aria-describedby={describedBy(descriptionId, problems.description)}
          onChange={(event) => change({ description: event.target.value })}
        />
      </Field>
      <Field
        id={urlId}
        label="Address"
        error={problems.url}
        hint="The server's https MCP address. Its domain is added to the outbound allowlist when you save."
      >
        <Input
          id={urlId}
          className="font-mono narrow:h-11"
          autoComplete="off"
          spellCheck={false}
          placeholder="https://mcp.example.com/mcp"
          value={values.url}
          aria-invalid={problems.url !== undefined}
          aria-describedby={describedBy(urlId, problems.url)}
          onChange={(event) => change({ url: event.target.value })}
        />
      </Field>
      <Field
        id={audienceId}
        label="Who can use its tools"
        hint="Admins manage every source whatever you choose here."
      >
        <Select
          items={audiences}
          value={values.audience}
          onValueChange={(pick) => {
            if (pick === "everyone" || pick === "admins") {
              change({ audience: pick })
            }
          }}
        >
          <SelectTrigger id={audienceId} className="w-full narrow:h-11">
            <SelectValue />
          </SelectTrigger>
          <SelectContent alignItemWithTrigger={false} align="start">
            {audiences.map((audience) => (
              <SelectItem key={audience.value} value={audience.value}>
                {audience.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </Field>
      <Field
        id={headerId}
        label="Secret header"
        error={problems.secretHeader}
        hint="The header that carries the secret, such as Authorization or X-Api-Key."
      >
        <Input
          id={headerId}
          className="font-mono narrow:h-11"
          autoComplete="off"
          spellCheck={false}
          placeholder="Authorization"
          value={values.secretHeader}
          disabled={values.clearSecret}
          aria-invalid={problems.secretHeader !== undefined}
          aria-describedby={describedBy(headerId, problems.secretHeader)}
          onChange={(event) => change({ secretHeader: event.target.value })}
        />
      </Field>
      <Field
        id={secretId}
        label="Secret"
        error={problems.secret}
        hint={secretHint}
      >
        <Input
          id={secretId}
          type="password"
          className="font-mono narrow:h-11"
          autoComplete="off"
          spellCheck={false}
          value={values.secret}
          disabled={values.clearSecret}
          aria-invalid={problems.secret !== undefined}
          aria-describedby={describedBy(secretId, problems.secret)}
          onChange={(event) => change({ secret: event.target.value })}
        />
      </Field>
      {mode === "edit" && secretStored ? (
        <div className="flex min-h-7 items-center gap-2 narrow:min-h-11">
          <Checkbox
            id={clearId}
            checked={values.clearSecret}
            onCheckedChange={(on) =>
              change({ clearSecret: on, secret: on ? "" : values.secret })
            }
          />
          <Label htmlFor={clearId} className="text-[13px]">
            Remove the stored secret
          </Label>
        </div>
      ) : null}
    </form>
  )
}
