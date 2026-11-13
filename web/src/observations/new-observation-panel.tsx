import { useId, useRef, type RefObject } from "react"
import { LoaderCircleIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { SidePanel } from "@/shell/side-panel"

import { NewObservationForm } from "./new-observation-form"
import { useCreateObservation } from "./queries"

type Props = {
  onClose: () => void
  trigger: RefObject<HTMLElement | null>
}

export function NewObservationPanel({ onClose, trigger }: Props) {
  const formId = `${useId()}-form`
  const titleInput = useRef<HTMLInputElement>(null)
  const create = useCreateObservation()

  return (
    <SidePanel
      title="New observation"
      initialFocus={titleInput}
      trigger={trigger}
      closeDisabled={create.isPending}
      onClose={onClose}
      footer={
        <>
          <Button
            variant="outline"
            disabled={create.isPending}
            onClick={onClose}
          >
            Cancel
          </Button>
          <Button
            type="submit"
            form={formId}
            focusableWhenDisabled
            disabled={create.isPending}
            aria-busy={create.isPending}
          >
            {create.isPending ? (
              <LoaderCircleIcon
                aria-hidden="true"
                className="size-3.5 motion-safe:animate-spin"
              />
            ) : null}
            Create
          </Button>
        </>
      }
    >
      <NewObservationForm
        create={create}
        formId={formId}
        titleRef={titleInput}
      />
    </SidePanel>
  )
}
