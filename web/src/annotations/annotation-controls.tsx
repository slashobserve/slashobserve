import { PlusIcon } from "lucide-react"

import { Button } from "@/components/ui/button"

import { useAnnotationActions } from "./actions"
import { AnnotationFilter } from "./annotation-filter"

export function AnnotationControls() {
  const actions = useAnnotationActions()
  return (
    <>
      <AnnotationFilter />
      {actions?.canAdd ? (
        <Button
          variant="outline"
          onClick={() => actions.add({ fromMs: Date.now() })}
        >
          <PlusIcon aria-hidden="true" data-icon="inline-start" />
          Add annotation
        </Button>
      ) : null}
    </>
  )
}
