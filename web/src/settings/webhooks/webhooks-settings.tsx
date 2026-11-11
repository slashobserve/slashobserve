import { useState } from "react"
import { useQuery } from "@tanstack/react-query"

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Skeleton } from "@/components/ui/skeleton"
import { SettingsCard, SettingsProblem } from "@/settings/settings-card"

import type { ActivityType } from "./event-types"
import {
  useCreateWebhook,
  useUpdateWebhook,
  webhooksQuery,
  type Webhook,
} from "./queries"
import { SecretReveal } from "./secret-reveal"
import { WebhookForm } from "./webhook-form"
import { webhookProblemText, type WebhookInput } from "./webhook-problems"
import { WebhooksCard } from "./webhooks-card"

type Revealed = { webhookName: string; secret: string }

const formHint =
  "Each event is sent as a signed https request. Only events from now on are sent, and only ones its owner can read."

function inputOf(webhook: Webhook): WebhookInput {
  return {
    name: webhook.name,
    url: webhook.url,
    events: webhook.event_types as ActivityType[],
  }
}

export function WebhooksSettings() {
  const query = useQuery(webhooksQuery())
  const create = useCreateWebhook()
  const update = useUpdateWebhook()
  const [revealed, setRevealed] = useState<Revealed | null>(null)
  const [editing, setEditing] = useState<Webhook | null>(null)

  if (query.isError && query.data === undefined) {
    return (
      <SettingsCard title="Webhooks" hint={formHint}>
        <SettingsProblem
          title="Could not load the webhooks"
          detail={webhookProblemText(query.error)}
          retrying={query.isFetching}
          onRetry={() => void query.refetch()}
        />
      </SettingsCard>
    )
  }
  if (query.data === undefined) {
    return <Skeleton className="h-24 w-full max-w-[860px]" />
  }

  async function add(input: WebhookInput) {
    try {
      const created = await create.mutateAsync({
        name: input.name,
        url: input.url,
        event_types: input.events,
      })
      setRevealed({ webhookName: created.webhook.name, secret: created.secret })
      return true
    } catch {
      return false
    }
  }

  async function save(input: WebhookInput) {
    if (editing === null) return false
    try {
      await update.mutateAsync({
        id: editing.id,
        body: { name: input.name, url: input.url, event_types: input.events },
      })
      setEditing(null)
      return true
    } catch {
      return false
    }
  }

  function changeEditing(open: boolean) {
    if (open || update.isPending) return
    setEditing(null)
    update.reset()
  }

  return (
    <>
      {revealed === null ? null : (
        <SecretReveal
          webhookName={revealed.webhookName}
          secret={revealed.secret}
          onDone={() => setRevealed(null)}
        />
      )}
      {query.data.length > 0 ? (
        <WebhooksCard
          webhooks={query.data}
          onEdit={setEditing}
          onSecret={(webhookName, secret) =>
            setRevealed({ webhookName, secret })
          }
        />
      ) : null}
      <SettingsCard title="Add a webhook" hint={formHint}>
        <WebhookForm
          submitLabel="Add webhook"
          pending={create.isPending}
          serverError={create.isError ? create.error : null}
          onSubmit={add}
        />
      </SettingsCard>
      <Dialog open={editing !== null} onOpenChange={changeEditing}>
        <DialogContent className="max-h-[90dvh] overflow-y-auto sm:max-w-xl">
          <DialogHeader>
            <DialogTitle className="wrap-anywhere">
              Edit {editing?.name}
            </DialogTitle>
            <DialogDescription>
              Saving makes you the webhook&apos;s owner, so it sends what you
              can read. A new address keeps its place in the log.
            </DialogDescription>
          </DialogHeader>
          {editing === null ? null : (
            <WebhookForm
              key={editing.id}
              initial={inputOf(editing)}
              submitLabel="Save changes"
              pending={update.isPending}
              serverError={update.isError ? update.error : null}
              onSubmit={save}
              onCancel={() => changeEditing(false)}
            />
          )}
        </DialogContent>
      </Dialog>
    </>
  )
}
