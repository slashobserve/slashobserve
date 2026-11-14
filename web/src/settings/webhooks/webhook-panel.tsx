import { useId, useRef, type RefObject } from "react"
import { LoaderCircleIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { SidePanel } from "@/shell/side-panel"

import type { ActivityType } from "./event-types"
import { useCreateWebhook, useUpdateWebhook, type Webhook } from "./queries"
import { WebhookForm } from "./webhook-form"
import type { WebhookInput } from "./webhook-problems"

export const webhooksHint =
  "Each event is sent as a signed https request. Only events from now on are sent, and only ones its owner can read."

const editDescription =
  "Saving makes you the webhook's owner, so it sends what you can read. A new address keeps its place in the log."

const addNote =
  "The signing secret is shown once after you add the webhook and cannot be read again."

const editNote =
  "The signing secret stays the same. To change it, use Rotate secret in the row menu."

export type WebhookPanelMode =
  { kind: "add" } | { kind: "edit"; webhook: Webhook }

type Props = {
  mode: WebhookPanelMode
  onClose: () => void
  onCreated: (created: { webhookName: string; secret: string }) => void
  trigger: RefObject<HTMLElement | null>
}

function inputOf(webhook: Webhook): WebhookInput {
  return {
    name: webhook.name,
    url: webhook.url,
    events: webhook.event_types as ActivityType[],
  }
}

export function WebhookPanel({ mode, onClose, onCreated, trigger }: Props) {
  const formId = useId()
  const nameInput = useRef<HTMLInputElement>(null)
  const create = useCreateWebhook()
  const update = useUpdateWebhook()
  const active = mode.kind === "add" ? create : update
  const adding = mode.kind === "add"

  function submit(input: WebhookInput) {
    if (active.isPending) return
    const body = {
      name: input.name,
      url: input.url,
      event_types: input.events,
    }
    if (mode.kind === "add") {
      create.mutate(body, {
        onSuccess: (created) => {
          onCreated({
            webhookName: created.webhook.name,
            secret: created.secret,
          })
          onClose()
        },
      })
      return
    }
    update.mutate({ id: mode.webhook.id, body }, { onSuccess: onClose })
  }

  return (
    <SidePanel
      title={adding ? "Add a webhook" : `Edit ${mode.webhook.name}`}
      description={adding ? webhooksHint : editDescription}
      onClose={onClose}
      closeDisabled={active.isPending}
      initialFocus={nameInput}
      trigger={trigger}
      footer={
        <>
          <Button
            type="submit"
            form={formId}
            disabled={active.isPending}
            aria-busy={active.isPending}
          >
            {active.isPending ? (
              <LoaderCircleIcon
                aria-hidden="true"
                className="motion-safe:animate-spin"
              />
            ) : null}
            {adding ? "Add webhook" : "Save changes"}
          </Button>
          <Button
            variant="outline"
            onClick={onClose}
            disabled={active.isPending}
          >
            Cancel
          </Button>
        </>
      }
    >
      <WebhookForm
        key={adding ? "add" : mode.webhook.id}
        formId={formId}
        initial={adding ? undefined : inputOf(mode.webhook)}
        serverError={active.isError ? active.error : null}
        onSubmit={submit}
        firstInputRef={nameInput}
      />
      <p className="text-xs text-muted-foreground">
        {adding ? addNote : editNote}
      </p>
    </SidePanel>
  )
}
