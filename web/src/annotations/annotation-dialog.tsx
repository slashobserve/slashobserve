import { useId, useRef, useState, type FormEvent, type ReactNode } from "react"
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
import { zonedDateTime } from "@/lib/time"

import {
  validateEdit,
  validateNew,
  type Errors,
  type Fields,
  type Scope,
} from "./form"
import { useCreateAnnotation, useUpdateAnnotation } from "./mutations"
import type { Annotation } from "./queries"
import { zoneLabel } from "./time"

export type Draft =
  | {
      kind: "new"
      fromMs: number
      toMs: number | null
      panelId?: string | undefined
    }
  | { kind: "edit"; annotation: Annotation }

type DialogContext = {
  dashboardId: string
  zone: string | undefined
  panelTitle: (id: string) => string
  canGlobal: boolean
}

type Props = DialogContext & {
  draft: Draft
  onClose: () => void
}

export function AnnotationDialog({ draft, onClose, ...context }: Props) {
  return draft.kind === "new" ? (
    <NewAnnotation draft={draft} onClose={onClose} {...context} />
  ) : (
    <EditAnnotation annotation={draft.annotation} onClose={onClose} />
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
  error: string | null | undefined
  hint?: string
  children: ReactNode
}) {
  return (
    <div className="grid gap-2">
      <Label htmlFor={id}>{label}</Label>
      {children}
      {error ? (
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

function Footer({
  pending,
  failed,
  label,
  onCancel,
}: {
  pending: boolean
  failed: boolean
  label: string
  onCancel: () => void
}) {
  return (
    <DialogFooter>
      <Button
        type="button"
        variant="outline"
        aria-disabled={pending}
        onClick={onCancel}
      >
        Cancel
      </Button>
      <Button
        type="submit"
        focusableWhenDisabled
        disabled={pending}
        aria-busy={pending}
      >
        {pending ? (
          <LoaderCircleIcon
            aria-hidden="true"
            className="size-3.5 motion-safe:animate-spin"
          />
        ) : null}
        {pending ? "Saving" : failed ? "Try again" : label}
      </Button>
    </DialogFooter>
  )
}

function whole(ms: number, zone: string | undefined): string {
  return zonedDateTime(ms, zone)
}

function NewAnnotation({
  draft,
  onClose,
  dashboardId,
  zone,
  panelTitle,
  canGlobal,
}: Props & { draft: Extract<Draft, { kind: "new" }> }) {
  const ids = useId()
  const textRef = useRef<HTMLTextAreaElement>(null)
  const create = useCreateAnnotation()
  const [submitted, setSubmitted] = useState(false)
  const [fields, setFields] = useState<Fields>({
    text: "",
    tags: "",
    from: whole(draft.fromMs, zone),
    to: draft.toMs === null ? "" : whole(draft.toMs, zone),
    scope: draft.panelId === undefined ? "dashboard" : "panel",
  })
  const set = (patch: Partial<Fields>) => setFields({ ...fields, ...patch })
  const [openedAt] = useState(Date.now)
  const check = (now: number) =>
    validateNew(fields, { dashboardId, panelId: draft.panelId }, zone, now)
  const checked = check(openedAt)
  const errors: Errors = checked.ok || !submitted ? {} : checked.errors
  const scopes: { value: Scope; label: string }[] = [
    { value: "dashboard", label: "This dashboard" },
    ...(draft.panelId === undefined
      ? []
      : [
          {
            value: "panel" as const,
            label: `Panel: ${panelTitle(draft.panelId)}`,
          },
        ]),
    ...(canGlobal
      ? [{ value: "global" as const, label: "Every dashboard" }]
      : []),
  ]
  const pending = create.isPending

  function submit(event: FormEvent) {
    event.preventDefault()
    setSubmitted(true)
    const now = check(Date.now())
    if (!now.ok || pending) return
    create.mutate(now.body, {
      onSuccess: () => {
        toast.success("Annotation added")
        onClose()
      },
    })
  }

  return (
    <Dialog open onOpenChange={(open) => (open || pending ? null : onClose())}>
      <DialogContent className="sm:max-w-md" initialFocus={textRef}>
        <form className="grid gap-4" onSubmit={submit} noValidate>
          <DialogHeader>
            <DialogTitle>Add annotation</DialogTitle>
            <DialogDescription>
              Marks a moment or a range of time on the charts. It saves no
              version.
            </DialogDescription>
          </DialogHeader>
          <Field id={`${ids}-text`} label="What happened" error={errors.text}>
            <Textarea
              id={`${ids}-text`}
              ref={textRef}
              rows={3}
              value={fields.text}
              readOnly={pending}
              aria-invalid={errors.text !== undefined}
              aria-describedby={errors.text ? `${ids}-text-error` : undefined}
              onChange={(event) => set({ text: event.target.value })}
            />
          </Field>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field id={`${ids}-from`} label="From" error={errors.from}>
              <Input
                id={`${ids}-from`}
                autoComplete="off"
                className="tabular-nums"
                value={fields.from}
                readOnly={pending}
                aria-invalid={errors.from !== undefined}
                aria-describedby={errors.from ? `${ids}-from-error` : undefined}
                onChange={(event) => set({ from: event.target.value })}
              />
            </Field>
            <Field
              id={`${ids}-to`}
              label="To (optional)"
              error={errors.to}
              hint="Empty marks one moment."
            >
              <Input
                id={`${ids}-to`}
                autoComplete="off"
                className="tabular-nums"
                value={fields.to}
                readOnly={pending}
                aria-invalid={errors.to !== undefined}
                aria-describedby={
                  errors.to ? `${ids}-to-error` : `${ids}-to-hint`
                }
                onChange={(event) => set({ to: event.target.value })}
              />
            </Field>
          </div>
          <p className="-mt-2 text-sm text-muted-foreground">
            Times are in {zoneLabel(draft.fromMs, zone)}. Type 2026-11-18 14:22
            or now-15m.
          </p>
          <Field
            id={`${ids}-tags`}
            label="Tags"
            error={errors.tags}
            hint="Separate tags with commas or spaces, such as incident, db."
          >
            <Input
              id={`${ids}-tags`}
              autoComplete="off"
              value={fields.tags}
              readOnly={pending}
              aria-invalid={errors.tags !== undefined}
              aria-describedby={
                errors.tags ? `${ids}-tags-error` : `${ids}-tags-hint`
              }
              onChange={(event) => set({ tags: event.target.value })}
            />
          </Field>
          {scopes.length > 1 ? (
            <div className="grid gap-2">
              <Label id={`${ids}-scope`}>Shown on</Label>
              <Select
                items={scopes}
                value={fields.scope}
                onValueChange={(scope) => scope && set({ scope })}
              >
                <SelectTrigger
                  aria-labelledby={`${ids}-scope`}
                  className="w-full"
                >
                  <SelectValue />
                </SelectTrigger>
                <SelectContent alignItemWithTrigger={false} align="start">
                  {scopes.map((item) => (
                    <SelectItem key={item.value} value={item.value}>
                      {item.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          ) : null}
          {create.isError ? (
            <Alert variant="destructive">
              <AlertDescription>
                {loadProblemText(create.error)}
              </AlertDescription>
            </Alert>
          ) : null}
          <Footer
            pending={pending}
            failed={create.isError}
            label="Add"
            onCancel={onClose}
          />
        </form>
      </DialogContent>
    </Dialog>
  )
}

function EditAnnotation({
  annotation,
  onClose,
}: {
  annotation: Annotation
  onClose: () => void
}) {
  const ids = useId()
  const textRef = useRef<HTMLTextAreaElement>(null)
  const update = useUpdateAnnotation(annotation.id)
  const [submitted, setSubmitted] = useState(false)
  const [fields, setFields] = useState({
    text: annotation.text,
    tags: annotation.tags.join(", "),
  })
  const checked = validateEdit(fields)
  const errors: Errors = checked.ok || !submitted ? {} : checked.errors
  const unchanged =
    checked.ok &&
    checked.body.text === annotation.text &&
    checked.body.tags.join(",") === annotation.tags.join(",")
  const pending = update.isPending

  function submit(event: FormEvent) {
    event.preventDefault()
    setSubmitted(true)
    if (!checked.ok || pending) return
    if (unchanged) {
      onClose()
      return
    }
    update.mutate(checked.body, {
      onSuccess: () => {
        toast.success("Annotation updated")
        onClose()
      },
    })
  }

  return (
    <Dialog open onOpenChange={(open) => (open || pending ? null : onClose())}>
      <DialogContent className="sm:max-w-md" initialFocus={textRef}>
        <form className="grid gap-4" onSubmit={submit} noValidate>
          <DialogHeader>
            <DialogTitle>Edit annotation</DialogTitle>
            <DialogDescription>
              Changes the text and tags. Its time, dashboard and author stay as
              they are.
            </DialogDescription>
          </DialogHeader>
          <Field id={`${ids}-text`} label="What happened" error={errors.text}>
            <Textarea
              id={`${ids}-text`}
              ref={textRef}
              rows={3}
              value={fields.text}
              readOnly={pending}
              aria-invalid={errors.text !== undefined}
              aria-describedby={errors.text ? `${ids}-text-error` : undefined}
              onChange={(event) =>
                setFields({ ...fields, text: event.target.value })
              }
            />
          </Field>
          <Field
            id={`${ids}-tags`}
            label="Tags"
            error={errors.tags}
            hint="Separate tags with commas or spaces."
          >
            <Input
              id={`${ids}-tags`}
              autoComplete="off"
              value={fields.tags}
              readOnly={pending}
              aria-invalid={errors.tags !== undefined}
              aria-describedby={
                errors.tags ? `${ids}-tags-error` : `${ids}-tags-hint`
              }
              onChange={(event) =>
                setFields({ ...fields, tags: event.target.value })
              }
            />
          </Field>
          {update.isError ? (
            <Alert variant="destructive">
              <AlertDescription>
                {loadProblemText(update.error)}
              </AlertDescription>
            </Alert>
          ) : null}
          <Footer
            pending={pending}
            failed={update.isError}
            label="Save"
            onCancel={onClose}
          />
        </form>
      </DialogContent>
    </Dialog>
  )
}
