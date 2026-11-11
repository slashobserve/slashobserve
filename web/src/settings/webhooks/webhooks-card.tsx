import { Fragment, useState } from "react"
import { EllipsisIcon, LoaderCircleIcon } from "lucide-react"
import { toast } from "sonner"

import { StatusLabel } from "@/components/status-label"
import { Cell, HeaderCell, TableCard } from "@/components/table-card"
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
import { eventTypeSummary } from "./event-types"
import {
  useDeleteWebhook,
  useRotateSecret,
  useTestWebhook,
  useUpdateWebhook,
  type Webhook,
} from "./queries"
import { deliverySummary, webhookStatus } from "./status"
import { webhookProblemText } from "./webhook-problems"

type Props = {
  webhooks: Webhook[]
  onEdit: (webhook: Webhook) => void
  onSecret: (webhookName: string, secret: string) => void
}

type Confirming = { kind: "rotate" | "delete"; webhook: Webhook }

export function WebhooksCard({ webhooks, onEdit, onSecret }: Props) {
  const test = useTestWebhook()
  const update = useUpdateWebhook()
  const rotate = useRotateSecret()
  const remove = useDeleteWebhook()
  const [confirming, setConfirming] = useState<Confirming | null>(null)
  const [viewing, setViewing] = useState<string | null>(null)
  const busy = rotate.isPending || remove.isPending
  const failed = confirming?.kind === "rotate" ? rotate : remove

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
    remove.mutate(webhook.id, {
      onSuccess: () => {
        setConfirming(null)
        toast.success(`Deleted ${webhook.name}`)
      },
    })
  }

  return (
    <>
      <TableCard label="Webhooks" minWidth={820} className="mb-4">
        <thead>
          <tr>
            <HeaderCell>Name</HeaderCell>
            <HeaderCell>Address</HeaderCell>
            <HeaderCell>Events</HeaderCell>
            <HeaderCell>Status</HeaderCell>
            <HeaderCell>
              <span className="sr-only">Actions</span>
            </HeaderCell>
          </tr>
        </thead>
        <tbody>
          {webhooks.map((webhook) => {
            const status = webhookStatus(webhook)
            const testing = test.isPending && test.variables === webhook.id
            const open = viewing === webhook.id
            return (
              <Fragment key={webhook.id}>
                <tr>
                  <Cell className="font-medium wrap-anywhere">
                    {webhook.name}
                    <span className="block text-xs font-normal text-muted-foreground">
                      Owner: {webhook.owner.label}
                    </span>
                  </Cell>
                  <Cell className="font-mono text-xs wrap-anywhere">
                    {webhook.url}
                  </Cell>
                  <Cell>{eventTypeSummary(webhook.event_types)}</Cell>
                  <Cell>
                    <div className="flex flex-col gap-0.5">
                      <StatusLabel tone={status.tone}>
                        {status.label}
                      </StatusLabel>
                      {status.detail === null ? null : (
                        <span className="text-xs text-muted-foreground">
                          {status.detail}
                        </span>
                      )}
                    </div>
                  </Cell>
                  <Cell>
                    <div className="flex justify-end gap-2">
                      <Button
                        variant="outline"
                        size="sm"
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
                        aria-expanded={open}
                        aria-label={`Attempts of ${webhook.name}`}
                        onClick={() => setViewing(open ? null : webhook.id)}
                      >
                        Attempts
                      </Button>
                      <DropdownMenu>
                        <DropdownMenuTrigger
                          render={
                            <Button
                              variant="outline"
                              size="icon-sm"
                              aria-label={`More for ${webhook.name}`}
                            />
                          }
                        >
                          <EllipsisIcon aria-hidden="true" />
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end">
                          <DropdownMenuItem onClick={() => onEdit(webhook)}>
                            Edit
                          </DropdownMenuItem>
                          <DropdownMenuItem
                            onClick={() => toggleEnabled(webhook)}
                          >
                            {webhook.enabled ? "Disable" : "Enable"}
                          </DropdownMenuItem>
                          <DropdownMenuItem
                            onClick={() =>
                              setConfirming({ kind: "rotate", webhook })
                            }
                          >
                            Rotate secret
                          </DropdownMenuItem>
                          <DropdownMenuItem
                            variant="destructive"
                            onClick={() =>
                              setConfirming({ kind: "delete", webhook })
                            }
                          >
                            Delete
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </div>
                  </Cell>
                </tr>
                {open ? (
                  <tr>
                    <td colSpan={5} className="border-b bg-muted/30 p-0">
                      <DeliveriesList webhookId={webhook.id} />
                    </td>
                  </tr>
                ) : null}
              </Fragment>
            )
          })}
        </tbody>
      </TableCard>
      <AlertDialog open={confirming !== null} onOpenChange={changeConfirming}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle className="wrap-anywhere">
              {confirming?.kind === "rotate"
                ? "Rotate the secret of"
                : "Delete"}{" "}
              {confirming?.webhook.name}?
            </AlertDialogTitle>
            <AlertDialogDescription>
              {confirming?.kind === "rotate"
                ? "The old secret stops working with the next request, so update the receiver. You become the webhook's owner."
                : "The webhook and its recorded attempts are removed. Events already sent stay sent."}
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
              {confirming?.kind === "rotate"
                ? "Rotate secret"
                : "Delete webhook"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
