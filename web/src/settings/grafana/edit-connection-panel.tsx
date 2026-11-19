import { useId, useRef, type RefObject } from "react"
import { LoaderCircleIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { SidePanel } from "@/shell/side-panel"

import { ConnectionForm } from "./connection-form"
import {
  connectionPatch,
  grafanaProblemText,
  type ConnectionInput,
} from "./connection-problems"
import { testOutcome } from "./connection-test"
import { useUpdateConnection, type GrafanaConnection } from "./queries"

export function EditConnectionPanel({
  connection,
  onClose,
  trigger,
}: {
  connection: GrafanaConnection
  onClose: () => void
  trigger: RefObject<HTMLElement | null>
}) {
  const formId = useId()
  const nameInput = useRef<HTMLInputElement>(null)
  const update = useUpdateConnection()

  function save(input: ConnectionInput) {
    if (update.isPending) return
    const body = connectionPatch(input, connection)
    if (Object.keys(body).length === 0) {
      onClose()
      return
    }
    update.mutate(
      { id: connection.id, body },
      {
        onSuccess: (updated) => {
          testOutcome(updated, "Saved")
          onClose()
        },
      }
    )
  }

  return (
    <SidePanel
      title={`Edit ${connection.name}`}
      description="Change the name, or point it at another URL with its token."
      onClose={onClose}
      closeDisabled={update.isPending}
      initialFocus={nameInput}
      trigger={trigger}
      footer={
        <>
          <Button
            type="submit"
            form={formId}
            disabled={update.isPending}
            aria-busy={update.isPending}
          >
            {update.isPending ? (
              <LoaderCircleIcon
                aria-hidden="true"
                className="motion-safe:animate-spin"
              />
            ) : null}
            Save connection
          </Button>
          <Button
            variant="outline"
            onClick={onClose}
            disabled={update.isPending}
          >
            Cancel
          </Button>
        </>
      }
    >
      <ConnectionForm
        formId={formId}
        serverError={update.isError ? grafanaProblemText(update.error) : null}
        onSubmit={save}
        firstInputRef={nameInput}
        stored={{ name: connection.name, url: connection.url }}
      />
      <p className="text-xs text-muted-foreground">
        The stored token is kept unless you enter a new one, and it is never
        shown. Saving tests the connection again.
      </p>
    </SidePanel>
  )
}
