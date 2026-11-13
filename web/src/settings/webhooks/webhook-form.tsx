import { useId, useState, type FormEvent, type RefObject } from "react"
import { Link } from "@tanstack/react-router"
import { ChevronDownIcon } from "lucide-react"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { cn } from "@/lib/utils"

import {
  eventTypeGroups,
  eventTypeLabels,
  groupCountText,
  openGroupPrefixes,
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
  formId: string
  initial?: WebhookInput
  serverError: unknown
  onSubmit: (input: WebhookInput) => void
  firstInputRef: RefObject<HTMLInputElement | null>
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
            search={(prev) => ({
              ...prev,
              panel: undefined,
              webhook: undefined,
            })}
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
  formId,
  initial = empty,
  serverError,
  onSubmit,
  firstInputRef,
}: Props) {
  const baseId = useId()
  const [values, setValues] = useState(initial)
  const [problems, setProblems] = useState<WebhookProblems>({})
  const [open, setOpen] = useState(
    () => new Set(openGroupPrefixes(initial.events))
  )

  function submit(event: FormEvent) {
    event.preventDefault()
    const found = webhookProblems(values)
    setProblems(found)
    if (Object.keys(found).length > 0) return
    onSubmit(trimWebhook(values))
  }

  function change(next: Partial<WebhookInput>) {
    setValues((current) => ({ ...current, ...next }))
  }

  function toggleOpen(prefix: string) {
    setOpen((current) => {
      const next = new Set(current)
      if (next.has(prefix)) next.delete(prefix)
      else next.add(prefix)
      return next
    })
  }

  const nameId = `${baseId}-name`
  const urlId = `${baseId}-url`
  const eventsId = `${baseId}-events`

  return (
    <form
      id={formId}
      noValidate
      onSubmit={submit}
      className="flex flex-col gap-4"
    >
      <div className="flex flex-col gap-1.5">
        <Label htmlFor={nameId} className="text-[13px] font-medium">
          Name
        </Label>
        <Input
          id={nameId}
          ref={firstInputRef}
          className="narrow:h-11"
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
      <div className="flex flex-col gap-1.5">
        <Label htmlFor={urlId} className="text-[13px] font-medium">
          Address
        </Label>
        <Input
          id={urlId}
          autoComplete="off"
          spellCheck={false}
          placeholder="https://hooks.example.com/slashobserve"
          className="font-mono narrow:h-11"
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
        className="flex flex-col"
      >
        <legend className="mb-1.5 text-[13px] font-medium">
          Events to send
        </legend>
        <div className="flex flex-col divide-y rounded-control border">
          {eventTypeGroups.map((group) => {
            const chosen = group.types.filter((type) =>
              values.events.includes(type)
            )
            const all = chosen.length === group.types.length
            const expanded = open.has(group.prefix)
            const listId = `${baseId}-list-${group.prefix}`
            return (
              <div key={group.prefix}>
                <div className="flex min-h-9 items-center gap-2 px-2.5 narrow:min-h-11">
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
                  <span className="ml-auto text-xs text-muted-foreground">
                    {groupCountText(values.events, group)}
                  </span>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-sm"
                    className="narrow:size-11"
                    aria-expanded={expanded}
                    aria-controls={listId}
                    aria-label={`Show ${group.label} events`}
                    onClick={() => toggleOpen(group.prefix)}
                  >
                    <ChevronDownIcon
                      aria-hidden="true"
                      className={cn(expanded && "rotate-180")}
                    />
                  </Button>
                </div>
                {expanded ? (
                  <div
                    id={listId}
                    className="flex flex-col pr-2.5 pb-2 pl-[34px]"
                  >
                    {group.types.map((type: ActivityType) => (
                      <div
                        key={type}
                        className="flex min-h-7 items-center gap-2 narrow:min-h-11"
                      >
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
                ) : null}
              </div>
            )
          })}
        </div>
        {problems.events === undefined ? null : (
          <p
            id={`${eventsId}-error`}
            className="mt-1.5 text-xs text-destructive"
          >
            {problems.events}
          </p>
        )}
      </fieldset>
      {serverError === null || serverError === undefined ? null : (
        <ServerProblem error={serverError} />
      )}
    </form>
  )
}
