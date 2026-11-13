import { useRef, useState } from "react"
import { useQuery } from "@tanstack/react-query"
import { PlusIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import type { SettingsPanel } from "@/settings/sections"
import { SettingsCard, SettingsProblem } from "@/settings/settings-card"
import { SettingsSection } from "@/settings/settings-section"

import { webhooksQuery } from "./queries"
import { SecretReveal } from "./secret-reveal"
import { webhookProblemText } from "./webhook-problems"
import {
  WebhookPanel,
  webhooksHint,
  type WebhookPanelMode,
} from "./webhook-panel"
import { WebhooksCard } from "./webhooks-card"

type Revealed = { webhookName: string; secret: string }

type Props = {
  panel: SettingsPanel | null
  onPanel: (panel: SettingsPanel | null) => void
}

export function WebhooksSettings({ panel, onPanel }: Props) {
  const query = useQuery(webhooksQuery())
  const [revealed, setRevealed] = useState<Revealed | null>(null)
  const addButton = useRef<HTMLButtonElement>(null)
  const editing =
    panel?.kind === "webhook-edit"
      ? query.data?.find((webhook) => webhook.id === panel.webhookId)
      : undefined
  const panelMode: WebhookPanelMode | null =
    panel?.kind === "webhook-add"
      ? { kind: "add" }
      : editing === undefined
        ? null
        : { kind: "edit", webhook: editing }

  return (
    <SettingsSection
      title="Webhooks"
      description="Send workspace activity to an address of your choice as signed requests."
      action={
        <Button
          ref={addButton}
          variant={panelMode === null ? "default" : "outline"}
          className="narrow:h-11"
          onClick={() => onPanel({ kind: "webhook-add" })}
        >
          <PlusIcon aria-hidden="true" />
          Add webhook
        </Button>
      }
    >
      {revealed === null ? null : (
        <SecretReveal
          webhookName={revealed.webhookName}
          secret={revealed.secret}
          onDone={() => setRevealed(null)}
        />
      )}
      <SettingsCard title="Webhooks" hint={webhooksHint}>
        {query.isError && query.data === undefined ? (
          <SettingsProblem
            title="Could not load the webhooks"
            detail={webhookProblemText(query.error)}
            retrying={query.isFetching}
            onRetry={() => void query.refetch()}
          />
        ) : query.data === undefined ? (
          <div className="border-t p-4">
            <Skeleton className="h-16 w-full" />
          </div>
        ) : query.data.length === 0 ? (
          <p className="border-t px-4 py-3 text-[13px] text-muted-foreground">
            No webhooks yet.
          </p>
        ) : (
          <WebhooksCard
            webhooks={query.data}
            editingId={editing?.id ?? null}
            onEdit={(webhookId) => onPanel({ kind: "webhook-edit", webhookId })}
            onSecret={(webhookName, secret) =>
              setRevealed({ webhookName, secret })
            }
          />
        )}
      </SettingsCard>
      {panelMode === null ? null : (
        <WebhookPanel
          mode={panelMode}
          onClose={() => onPanel(null)}
          onCreated={setRevealed}
          trigger={addButton}
        />
      )}
    </SettingsSection>
  )
}
