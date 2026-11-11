import { useId, useRef, useState, type FormEvent } from "react"
import { Link } from "@tanstack/react-router"
import { LoaderCircleIcon } from "lucide-react"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"

import {
  eventTypeGroups,
  eventTypeLabels,
  toggleEventType,
  toggleGroup,
  type ActivityType,
} from "./event-types"
import {
  isDomainNotAllowed,
  trimWebhook,
  webhookProblems,
  webhookProblemText,
  type WebhookInput,
  type WebhookProblems,
} from "./webhook-problems"

type Props = {
  initial?: WebhookInput
  submitLabel: string
  pending: boolean
  serverError: unknown
  onSubmit: (input: WebhookInput) => Promise<boolean>
  onCancel?: () => void
}

const empty: WebhookInput = { name: "", url: "", events: [] }

function ServerProblem({ error }: { error: unknown }) {
  if (isDomainNotAllowed(error)) {
    return (
      <Alert variant="destructive">
        <AlertDescription>
          This domain is not on the outbound allowlist, and a webhook never adds
          it. An Admin can allow it in the{" "}
          <Link
            to="/settings"
            search={true}
            hash="outbound-access"
            className="underline underline-offset-3"
          >
            Outbound access
          </Link>{" "}
          section.
        </AlertDescription>
      </Alert>
    )
  }
  return (
    <Alert variant="destructive">
      <AlertDescription>{webhookProblemText(error)}</AlertDescription>
    </Alert>
  )
}

export function WebhookForm({
  initial = empty,
  submitLabel,
  pending,
  serverError,
  onSubmit,
  onCancel,
}: Props) {
  const baseId = useId()
  const [values, setValues] = useState(initial)
  const [problems, setProblems] = useState<WebhookProblems>({})
  const firstInput = useRef<HTMLInputElement>(null)

  async function submit(event: FormEvent) {
    event.preventDefault()
    if (pending) return
    const found = webhookProblems(values)
    setProblems(found)
    if (Object.keys(found).length > 0) return
    if (await onSubmit(trimWebhook(values))) {
      setValues(initial)
      firstInput.current?.focus()
    }
  }

  function change(next: Partial<WebhookInput>) {
    setValues((current) => ({ ...current, ...next }))
  }

  const nameId = `${baseId}-name`
  const urlId = `${baseId}-url`
  const eventsId = `${baseId}-events`

  return (
    <form
      noValidate
      onSubmit={(event) => void submit(event)}
      className="flex flex-col gap-3 px-4 py-3"
    >
      <div className="flex flex-col gap-1">
        <Label htmlFor={nameId} className="text-[13px]">
          Name
        </Label>
        <Input
          id={nameId}
          ref={firstInput}
          autoComplete="off"
          placeholder="Failed runs"
          value={values.name}
          aria-invalid={problems.name !== undefined}
          aria-describedby={
            problems.name === undefined ? undefined : `${nameId}-error`
          }
          onChange={(event) => change({ name: event.target.value })}
        />
        {problems.name === undefined ? null : (
          <p id={`${nameId}-error`} className="text-xs text-destructive">
            {problems.name}
          </p>
        )}
      </div>
      <div className="flex flex-col gap-1">
        <Label htmlFor={urlId} className="text-[13px]">
          Address
        </Label>
        <Input
          id={urlId}
          autoComplete="off"
          spellCheck={false}
          placeholder="https://hooks.example.com/slashobserve"
          className="font-mono"
          value={values.url}
          aria-invalid={problems.url !== undefined}
          aria-describedby={
            problems.url === undefined ? undefined : `${urlId}-error`
          }
          onChange={(event) => change({ url: event.target.value })}
        />
        {problems.url === undefined ? null : (
          <p id={`${urlId}-error`} className="text-xs text-destructive">
            {problems.url}
          </p>
        )}
      </div>
      <fieldset
        aria-describedby={
          problems.events === undefined ? undefined : `${eventsId}-error`
        }
        className="flex flex-col gap-2"
      >
        <legend className="mb-1 text-[13px] font-medium">Events to send</legend>
        {eventTypeGroups.map((group) => {
          const chosen = group.types.filter((type) =>
            values.events.includes(type)
          )
          const all = chosen.length === group.types.length
          return (
            <div key={group.prefix} className="flex flex-col gap-1.5">
              <div className="flex items-center gap-2">
                <Checkbox
                  id={`${baseId}-group-${group.prefix}`}
                  checked={all}
                  indeterminate={chosen.length > 0 && !all}
                  onCheckedChange={(on) =>
                    change({ events: toggleGroup(values.events, group, on) })
                  }
                />
                <Label
                  htmlFor={`${baseId}-group-${group.prefix}`}
                  className="text-[13px] font-medium"
                >
                  {group.label}
                </Label>
              </div>
              <div className="ml-6 flex flex-wrap gap-x-5 gap-y-1.5">
                {group.types.map((type: ActivityType) => (
                  <div key={type} className="flex items-center gap-2">
                    <Checkbox
                      id={`${baseId}-${type}`}
                      checked={values.events.includes(type)}
                      onCheckedChange={(on) =>
                        change({
                          events: toggleEventType(values.events, type, on),
                        })
                      }
                    />
                    <Label
                      htmlFor={`${baseId}-${type}`}
                      className="text-[13px]"
                    >
                      {eventTypeLabels[type]}
                    </Label>
                  </div>
                ))}
              </div>
            </div>
          )
        })}
        {problems.events === undefined ? null : (
          <p id={`${eventsId}-error`} className="text-xs text-destructive">
            {problems.events}
          </p>
        )}
      </fieldset>
      <div className="flex flex-wrap items-center gap-2">
        <Button type="submit" disabled={pending} aria-busy={pending}>
          {pending ? (
            <LoaderCircleIcon
              aria-hidden="true"
              className="motion-safe:animate-spin"
            />
          ) : null}
          {submitLabel}
        </Button>
        {onCancel === undefined ? null : (
          <Button type="button" variant="outline" onClick={onCancel}>
            Cancel
          </Button>
        )}
      </div>
      {serverError === null || serverError === undefined ? null : (
        <ServerProblem error={serverError} />
      )}
    </form>
  )
}
