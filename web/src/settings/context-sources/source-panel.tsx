import { useId, useRef, type RefObject } from "react"
import { LoaderCircleIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { SidePanel } from "@/shell/side-panel"

import { createBody, updateBody, type SourceInput } from "./problems"
import {
  useCreateContextSource,
  useUpdateContextSource,
  type ContextSource,
} from "./queries"
import { SourceForm } from "./source-form"

export const contextSourcesHint =
  "An MCP server the agent may read from. Registering one asks the server nothing: discover its tools, then approve the ones the agent may call."

export type SourcePanelMode =
  { kind: "add" } | { kind: "edit"; source: ContextSource }

type Props = {
  mode: SourcePanelMode
  onClose: () => void
  trigger: RefObject<HTMLElement | null>
}

const addNote =
  "Only read-only tools can be approved, and each one is pinned by a hash. If the server later describes an approved tool differently, the agent stops using it until you approve it again."

const editNote =
  "A new address drops the approved tools, so approvals never carry over to another server. A new host needs the secret again, or removes it."

export function SourcePanel({ mode, onClose, trigger }: Props) {
  const formId = useId()
  const firstInput = useRef<HTMLInputElement>(null)
  const create = useCreateContextSource()
  const update = useUpdateContextSource()
  const adding = mode.kind === "add"
  const active = adding ? create : update

  function submit(input: SourceInput) {
    if (active.isPending) return
    if (mode.kind === "add") {
      create.mutate(createBody(input), { onSuccess: onClose })
      return
    }
    const body = updateBody(mode.source, input)
    if (Object.keys(body).length === 0) {
      onClose()
      return
    }
    update.mutate({ uid: mode.source.uid, body }, { onSuccess: onClose })
  }

  return (
    <SidePanel
      title={adding ? "Add a context source" : `Edit ${mode.source.name}`}
      description={
        adding
          ? "Register an MCP server the agent can use for outside context."
          : "Change where this source is and who can use its tools."
      }
      onClose={onClose}
      closeDisabled={active.isPending}
      initialFocus={firstInput}
      trigger={trigger}
      footer={
        <>
          <Button
            type="submit"
            form={formId}
            disabled={active.isPending}
            aria-busy={active.isPending}
          >
            {active.isPending ? (
              <LoaderCircleIcon
                aria-hidden="true"
                className="motion-safe:animate-spin"
              />
            ) : null}
            {adding ? "Add source" : "Save changes"}
          </Button>
          <Button
            variant="outline"
            onClick={onClose}
            disabled={active.isPending}
          >
            Cancel
          </Button>
        </>
      }
    >
      <SourceForm
        key={adding ? "add" : mode.source.uid}
        formId={formId}
        mode={adding ? "add" : "edit"}
        source={adding ? undefined : mode.source}
        serverError={active.isError ? active.error : null}
        onSubmit={submit}
        firstInputRef={firstInput}
      />
      <p className="text-xs text-muted-foreground">
        {adding ? addNote : editNote}
      </p>
    </SidePanel>
  )
}
