import { useId, useRef, useState } from "react"
import { createFileRoute } from "@tanstack/react-router"
import { PlusIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Separator } from "@/components/ui/separator"
import { useDocumentTitle } from "@/lib/document-title"
import { ConnectClients } from "@/service-accounts/connect-clients"
import { NewKeyDialog } from "@/service-accounts/new-key-dialog"
import type { ServiceAccount } from "@/service-accounts/queries"
import { ServiceAccountList } from "@/service-accounts/service-account-list"

export const Route = createFileRoute("/_authed/_shell/keys")({
  staticData: { crumbs: [{ label: "API keys" }] },
  component: KeysPage,
})

type DialogState = { open: boolean; account: ServiceAccount | null }

function KeysPage() {
  const headingId = useId()
  const returnFocus = useRef<HTMLElement | null>(null)
  const headerButton = useRef<HTMLButtonElement>(null)
  const [dialog, setDialog] = useState<DialogState>({
    open: false,
    account: null,
  })

  useDocumentTitle("API keys - SlashObserve")

  function openCreate(account: ServiceAccount | null, trigger: HTMLElement) {
    returnFocus.current = trigger
    setDialog({ open: true, account })
  }

  return (
    <>
      <div className="mb-6 flex flex-wrap items-start justify-between gap-x-6 gap-y-3">
        <div className="flex max-w-xl flex-col gap-1">
          <h1 id={headingId} className="text-2xl font-semibold">
            API keys
          </h1>
          <p className="text-sm text-muted-foreground">
            Service accounts sign in the observe CLI, MCP clients and scripts.
            Anyone signed in to this server can create and revoke their keys.
          </p>
        </div>
        <Button
          ref={headerButton}
          onClick={(event) => openCreate(null, event.currentTarget)}
        >
          <PlusIcon aria-hidden="true" />
          New service account
        </Button>
      </div>
      <ServiceAccountList labelledBy={headingId} onCreateKey={openCreate} />
      <Separator className="my-8" />
      <ConnectClients />
      <NewKeyDialog
        open={dialog.open}
        account={dialog.account}
        finalFocus={() =>
          returnFocus.current?.isConnected
            ? returnFocus.current
            : headerButton.current
        }
        onOpenChange={(open) => setDialog((state) => ({ ...state, open }))}
      />
    </>
  )
}
