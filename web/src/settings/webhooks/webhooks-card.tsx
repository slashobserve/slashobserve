import { useMemo, useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { EllipsisIcon, LoaderCircleIcon } from "lucide-react"
import { toast } from "sonner"

import { webhookKeys } from "@/api/keys"
import { RelativeTime, useNow } from "@/components/relative-time"
import { StatusLabel } from "@/components/status-label"
import { DataGrid, type DataGridColumn } from "@/components/data-grid/data-grid"
import { Alert, AlertDescription } from "@/components/ui/alert"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"

import { DeliveriesList } from "./deliveries-list"
import {
  eventTypeLabels,
  eventTypeSummary,
  type ActivityType,
} from "./event-types"
import {
  useDeleteWebhook,
  useRotateSecret,
  useSkipWebhookEvent,
  useTestWebhook,
  useUpdateWebhook,
  type Webhook,
} from "./queries"
import { deliverySummary, webhookStatus } from "./status"
import { formatLabels, webhookProblemText } from "./webhook-problems"

type Props = {
  webhooks: Webhook[]
  editingId: string | null
  meId: string
  onEdit: (webhookId: string) => void
  onSecret: (webhookName: string, secret: string) => void
}

type Confirming = {
  kind: "rotate" | "delete" | "skip" | "own"
  webhook: Webhook
}

const confirmTitles: Record<Confirming["kind"], string> = {
  rotate: "Rotate the secret of",
  skip: "Skip the stuck event of",
  own: "Make yourself the owner of",
  delete: "Delete",
}

const confirmActions: Record<Confirming["kind"], string> = {
  rotate: "Rotate secret",
  skip: "Skip event",
  own: "Make me the owner",
  delete: "Delete webhook",
}

const confirmBodies: Record<Confirming["kind"], (webhook: Webhook) => string> =
  {
    rotate: () =>
      "The old secret stops working with the next request, so update the receiver. The owner stays the same.",
    skip: () =>
      "The oldest event this webhook is waiting to send is never sent to the receiver. The events behind it go out next, in order. Attempts lists the skipped event.",
    own: (webhook) =>
      `${webhook.owner.label} owns this webhook now. After this it sends only what you can read, including events in your personal folder, and no longer what ${webhook.owner.label} could read.`,
    delete: () =>
      "The webhook and its recorded attempts are removed. Events already sent stay sent.",
  }

function NextAttempt({ iso }: { iso: string }) {
  const now = useNow()
  if (Date.parse(iso) <= now) return <>next attempt is due now</>
  return (
    <>
      next attempt <RelativeTime iso={iso} />
    </>
  )
}

export function WebhooksCard({
  webhooks,
  editingId,
  meId,
  onEdit,
  onSecret,
}: Props) {
  const test = useTestWebhook()
  const update = useUpdateWebhook()
  const takeOwnership = useUpdateWebhook()
  const rotate = useRotateSecret()
  const remove = useDeleteWebhook()
  const skip = useSkipWebhookEvent()
  const queryClient = useQueryClient()
  const [confirming, setConfirming] = useState<Confirming | null>(null)
  const [viewing, setViewing] = useState<string | null>(null)
  const openDetail = useMemo(
    () => new Set(viewing === null ? [] : [viewing]),
    [viewing]
  )
  const busy =
    rotate.isPending ||
    remove.isPending ||
    skip.isPending ||
    takeOwnership.isPending
  const failed = {
    rotate,
    skip,
    own: takeOwnership,
    delete: remove,
  }[confirming?.kind ?? "delete"]

  function runTest(webhook: Webhook) {
    test.mutate(webhook.id, {
      onSuccess: ({ delivery }) => {
        const text = `${webhook.name}: ${deliverySummary(delivery)}`
        if (delivery.outcome === "delivered") toast.success(text)
        else toast.error(text)
      },
      onError: (error) => toast.error(webhookProblemText(error)),
    })
  }

  function toggleEnabled(webhook: Webhook) {
    update.mutate(
      { id: webhook.id, body: { enabled: !webhook.enabled } },
      { onError: (error) => toast.error(webhookProblemText(error)) }
    )
  }

  function changeConfirming(open: boolean) {
    if (open || busy) return
    setConfirming(null)
    rotate.reset()
    remove.reset()
    skip.reset()
    takeOwnership.reset()
  }

  function toggleAttempts(webhook: Webhook, open: boolean) {
    setViewing(open ? null : webhook.id)
    void queryClient.invalidateQueries({ queryKey: webhookKeys.list() })
  }

  function confirm() {
    if (confirming === null) return
    const { kind, webhook } = confirming
    if (kind === "rotate") {
      rotate.mutate(webhook.id, {
        onSuccess: (rotated) => {
          setConfirming(null)
          onSecret(rotated.webhook.name, rotated.secret)
        },
      })
      return
    }
    if (kind === "own") {
      takeOwnership.mutate(
        { id: webhook.id, body: { owner: "me" } },
        {
          onSuccess: () => {
            setConfirming(null)
            toast.success(`You now own ${webhook.name}`)
          },
        }
      )
      return
    }
    if (kind === "skip") {
      skip.mutate(webhook.id, {
        onSuccess: ({ skipped }) => {
          setConfirming(null)
          toast.success(
            `Skipped ${eventTypeLabels[skipped.event_type as ActivityType] ?? skipped.event_type} on ${webhook.name}`
          )
        },
      })
      return
    }
    remove.mutateAsync(webhook.id).then(
      () => {
        setConfirming(null)
        toast.success(`Deleted ${webhook.name}`)
      },
      () => {}
    )
  }

  const columns: DataGridColumn<Webhook>[] = [
    {
      id: "name",
      header: "Name",
      minWidth: 150,
      autoHeight: true,
      cell: (webhook) => (
        <div className="py-2 font-medium wrap-anywhere">
          {webhook.name}
          <span className="block text-xs font-normal text-muted-foreground">
            Owner: {webhook.owner.label}
          </span>
        </div>
      ),
    },
    {
      id: "address",
      header: "Address",
      minWidth: 200,
      flex: 1.5,
      autoHeight: true,
      cell: (webhook) => (
        <div className="py-2 text-xs wrap-anywhere">
          <span className="font-mono">{webhook.url}</span>
          {webhook.format === "generic" ? null : (
            <span className="block text-muted-foreground">
              Sends {formatLabels[webhook.format]}
            </span>
          )}
        </div>
      ),
    },
    {
      id: "events",
      header: "Events",
      minWidth: 140,
      autoHeight: true,
      cell: (webhook) => (
        <span className="py-2 wrap-break-word">
          {eventTypeSummary(webhook.event_types)}
        </span>
      ),
    },
    {
      id: "status",
      header: "Status",
      minWidth: 160,
      autoHeight: true,
      cell: (webhook) => {
        const status = webhookStatus(webhook)
        return (
          <div className="flex flex-col gap-0.5 py-2">
            <StatusLabel tone={status.tone}>{status.label}</StatusLabel>
            {status.detail === null ? null : (
              <span className="text-xs text-muted-foreground">
                {status.detail}
              </span>
            )}
            {status.waiting === null ? null : (
              <span className="text-xs text-muted-foreground">
                {status.waiting}
                {webhook.enabled ? (
                  <>
                    , <NextAttempt iso={webhook.next_attempt_at} />
                  </>
                ) : null}
              </span>
            )}
          </div>
        )
      },
    },
    {
      id: "actions",
      header: "Actions",
      action: true,
      align: "right",
      width: 250,
      cell: (webhook) => {
        const testing = test.isPending && test.variables === webhook.id
        const open = viewing === webhook.id
        return (
          <div className="flex justify-end gap-2">
            <Button
              variant="outline"
              size="sm"
              className="narrow:h-11"
              disabled={testing}
              focusableWhenDisabled
              aria-busy={testing}
              aria-label={`Test ${webhook.name}`}
              onClick={() => runTest(webhook)}
            >
              {testing ? (
                <LoaderCircleIcon
                  aria-hidden="true"
                  className="motion-safe:animate-spin"
                />
              ) : null}
              Test
            </Button>
            <Button
              variant="outline"
              size="sm"
              className="narrow:h-11"
              aria-expanded={open}
              aria-label={`Attempts of ${webhook.name}`}
              onClick={() => toggleAttempts(webhook, open)}
            >
              Attempts
            </Button>
            <DropdownMenu>
              <DropdownMenuTrigger
                render={
                  <Button
                    variant="outline"
                    size="icon-sm"
                    className="narrow:size-11"
                    aria-label={`More for ${webhook.name}`}
                  />
                }
              >
                <EllipsisIcon aria-hidden="true" />
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem onClick={() => onEdit(webhook.id)}>
                  Edit
                </DropdownMenuItem>
                <DropdownMenuItem onClick={() => toggleEnabled(webhook)}>
                  {webhook.enabled ? "Disable" : "Enable"}
                </DropdownMenuItem>
                {webhook.enabled && webhook.consecutive_failures > 0 ? (
                  <DropdownMenuItem
                    onClick={() => setConfirming({ kind: "skip", webhook })}
                  >
                    Skip stuck event
                  </DropdownMenuItem>
                ) : null}
                {webhook.owner.id === meId ? null : (
                  <DropdownMenuItem
                    onClick={() => setConfirming({ kind: "own", webhook })}
                  >
                    Make me the owner
                  </DropdownMenuItem>
                )}
                <DropdownMenuItem
                  onClick={() => setConfirming({ kind: "rotate", webhook })}
                >
                  Rotate secret
                </DropdownMenuItem>
                <DropdownMenuItem
                  variant="destructive"
                  onClick={() => setConfirming({ kind: "delete", webhook })}
                >
                  Delete
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        )
      },
    },
  ]

  return (
    <>
      <DataGrid
        label="Webhooks"
        rows={webhooks}
        columns={columns}
        rowId={(webhook) => webhook.id}
        highlightedRowId={editingId}
        detail={{
          open: openDetail,
          render: (webhook) => <DeliveriesList webhookId={webhook.id} />,
        }}
        bare
        className="border-t"
      />
      <AlertDialog open={confirming !== null} onOpenChange={changeConfirming}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle className="wrap-anywhere">
              {confirmTitles[confirming?.kind ?? "delete"]}{" "}
              {confirming?.webhook.name}?
            </AlertDialogTitle>
            <AlertDialogDescription>
              {confirming === null
                ? null
                : confirmBodies[confirming.kind](confirming.webhook)}
            </AlertDialogDescription>
          </AlertDialogHeader>
          {failed.isError ? (
            <Alert variant="destructive">
              <AlertDescription>
                {webhookProblemText(failed.error)}
              </AlertDescription>
            </Alert>
          ) : null}
          <AlertDialogFooter>
            <AlertDialogCancel aria-disabled={busy}>Cancel</AlertDialogCancel>
            <AlertDialogAction
              variant={
                confirming?.kind === "delete" ? "destructive" : "default"
              }
              disabled={busy}
              focusableWhenDisabled
              aria-busy={busy}
              onClick={confirm}
            >
              {busy ? (
                <LoaderCircleIcon
                  aria-hidden="true"
                  className="motion-safe:animate-spin"
                />
              ) : null}
              {confirmActions[confirming?.kind ?? "delete"]}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
