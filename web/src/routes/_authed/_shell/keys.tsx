import { useEffect, useRef, useState } from "react"
import { createFileRoute } from "@tanstack/react-router"
import { PlusIcon } from "lucide-react"

import { PageHeader } from "@/components/page-header"
import { Button } from "@/components/ui/button"
import { useDocumentTitle } from "@/lib/document-title"
import { ConnectClients } from "@/service-accounts/connect-clients"
import { CreateKeyPanel } from "@/service-accounts/create-key-panel"
import { KeysTable } from "@/service-accounts/keys-table"

export const Route = createFileRoute("/_authed/_shell/keys")({
  staticData: { crumbs: [{ label: "API keys" }] },
  component: KeysPage,
})

function KeysPage() {
  const createButton = useRef<HTMLButtonElement>(null)
  const nameInput = useRef<HTMLInputElement>(null)
  const copyButton = useRef<HTMLButtonElement>(null)
  const [panelOpen, setPanelOpen] = useState(false)
  const wasOpen = useRef(false)

  useDocumentTitle("API keys - SlashObserve")

  useEffect(() => {
    const closed = wasOpen.current && !panelOpen
    wasOpen.current = panelOpen
    if (closed && document.activeElement === document.body) {
      createButton.current?.focus()
    }
  }, [panelOpen])

  function openPanel() {
    if (!panelOpen) {
      setPanelOpen(true)
      return
    }
    const target = nameInput.current?.disabled
      ? copyButton.current
      : nameInput.current
    target?.focus()
  }

  return (
    <>
      <PageHeader
        title="API keys"
        description="Keys let Claude Code, OpenCode and the observe CLI use this server over MCP and the API."
        actions={
          <Button ref={createButton} onClick={openPanel}>
            <PlusIcon aria-hidden="true" />
            Create key
          </Button>
        }
      />
      <div className="mb-5">
        <KeysTable onCreate={openPanel} revokeFocus={createButton} />
      </div>
      <ConnectClients />
      {panelOpen ? (
        <CreateKeyPanel
          nameInput={nameInput}
          copyButton={copyButton}
          onClose={() => setPanelOpen(false)}
        />
      ) : null}
    </>
  )
}
