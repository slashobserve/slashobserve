import { useId, useRef, type RefObject } from "react"
import { LoaderCircleIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import { SidePanel } from "@/shell/side-panel"

import type { DatasourceType } from "../draft"
import { SourceKindSelect } from "../source-kind-select"

import { createBody, updateBody, type SourceInput } from "./problems"
import {
  useCreateMCPSource,
  useUpdateMCPSource,
  type MCPSource,
} from "./queries"
import { SourceForm } from "./source-form"

export type SourcePanelMode =
  { kind: "add" } | { kind: "edit"; source: MCPSource }

type Props = {
  mode: SourcePanelMode
  onClose: () => void
  trigger: RefObject<HTMLElement | null>
  onAdded?: (uid: string) => void
  onPickNative?: (type: DatasourceType) => void
}

const addNote =
  "Only read-only tools can be approved, and each one is pinned by a hash. If the server later describes an approved tool differently, the agent stops using it until you approve it again."

const editNote =
  "A new address drops the approved tools, so approvals never carry over to another server. A new host needs the secret again, or removes it."

function submitLabel(adding: boolean, pending: boolean) {
  if (adding) return pending ? "Adding source..." : "Add source"
  return pending ? "Saving..." : "Save changes"
}

export function SourcePanel({
  mode,
  onClose,
  trigger,
  onAdded,
  onPickNative,
}: Props) {
  const formId = useId()
  const kindId = useId()
  const firstInput = useRef<HTMLInputElement>(null)
  const create = useCreateMCPSource()
  const update = useUpdateMCPSource()
  const adding = mode.kind === "add"
  const active = adding ? create : update

  function submit(input: SourceInput) {
    if (active.isPending) return
    if (mode.kind === "add") {
      create.mutate(createBody(input), {
        onSuccess: () => {
          onAdded?.(input.uid)
          onClose()
        },
      })
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
      title={adding ? "Add an MCP server" : `Edit ${mode.source.name}`}
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
            {submitLabel(adding, active.isPending)}
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
      {onPickNative === undefined ? null : (
        <div className="mb-4 flex flex-col gap-1.5">
          <Label htmlFor={kindId} className="text-[13px] font-medium">
            Type
          </Label>
          <SourceKindSelect
            id={kindId}
            value="mcp"
            onChange={(kind) => {
              if (kind !== "mcp") onPickNative(kind)
            }}
          />
        </div>
      )}
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
