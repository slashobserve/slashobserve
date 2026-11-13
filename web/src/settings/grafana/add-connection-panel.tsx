import { useId, useRef, type RefObject } from "react"
import { LoaderCircleIcon } from "lucide-react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import { SidePanel } from "@/shell/side-panel"

import { ConnectionForm } from "./connection-form"
import { grafanaProblemText, type ConnectionInput } from "./connection-problems"
import { useCreateConnection } from "./queries"

export const connectionsHint =
  "A Grafana URL and a service account token with the Viewer role on the folders to import."

export function AddConnectionPanel({
  onClose,
  trigger,
}: {
  onClose: () => void
  trigger: RefObject<HTMLElement | null>
}) {
  const formId = useId()
  const nameInput = useRef<HTMLInputElement>(null)
  const create = useCreateConnection()

  function add(input: ConnectionInput) {
    if (create.isPending) return
    create.mutate(input, {
      onSuccess: () => {
        toast.success(`Added ${input.name}`)
        onClose()
      },
    })
  }

  return (
    <SidePanel
      title="Add a connection"
      description={connectionsHint}
      onClose={onClose}
      closeDisabled={create.isPending}
      initialFocus={nameInput}
      trigger={trigger}
      footer={
        <>
          <Button
            type="submit"
            form={formId}
            disabled={create.isPending}
            aria-busy={create.isPending}
          >
            {create.isPending ? (
              <LoaderCircleIcon
                aria-hidden="true"
                className="motion-safe:animate-spin"
              />
            ) : null}
            Add connection
          </Button>
          <Button
            variant="outline"
            onClick={onClose}
            disabled={create.isPending}
          >
            Cancel
          </Button>
        </>
      }
    >
      <ConnectionForm
        formId={formId}
        serverError={create.isError ? grafanaProblemText(create.error) : null}
        onSubmit={add}
        firstInputRef={nameInput}
      />
      <p className="text-xs text-muted-foreground">
        The token is stored encrypted and is never shown again.
      </p>
    </SidePanel>
  )
}
