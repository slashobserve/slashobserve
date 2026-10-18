import { useState, type ReactNode } from "react"
import { useIsMutating } from "@tanstack/react-query"
import { LayoutGridIcon, XIcon } from "lucide-react"

import { dashboardKeys } from "@/api/keys"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import type { Dashboard } from "@/spec"

import {
  editLayoutButtonId,
  focusEditLayoutButton,
  focusWhenIdle,
  useIsDirty,
  useLayoutStore,
} from "./layout-store"
import { SaveLayoutDialog } from "./save-layout-dialog"
import { DiscardLayoutDialog } from "./unsaved-changes-guard"

type Props = {
  dashboardId: string
  version: number
  spec: Dashboard
  pinned: boolean
  canEdit: boolean
}

function focusFirstMoveHandle() {
  focusWhenIdle(() => document.querySelector("button.panel-drag-handle"))
}

export function EditControls({
  dashboardId,
  version,
  spec,
  pinned,
  canEdit,
}: Props) {
  const sessionOpen = useLayoutStore(
    (state) => state.session?.dashboardId === dashboardId
  )
  const editing = sessionOpen && !pinned
  const start = useLayoutStore((state) => state.start)
  const end = useLayoutStore((state) => state.end)
  const dirty = useIsDirty()
  const [saveOpen, setSaveOpen] = useState(false)
  const [discardOpen, setDiscardOpen] = useState(false)

  function cancel() {
    if (dirty) {
      setDiscardOpen(true)
      return
    }
    end()
    focusEditLayoutButton()
  }

  let controls: ReactNode = null
  if (editing) {
    controls = (
      <>
        {dirty ? (
          <Badge variant="secondary" className="max-sm:hidden">
            Unsaved changes
          </Badge>
        ) : null}
        <Button
          variant="ghost"
          className="max-sm:w-8 max-sm:px-0"
          onClick={cancel}
        >
          <XIcon className="sm:hidden" />
          <span className="max-sm:sr-only">Cancel</span>
        </Button>
        <Button disabled={!dirty} onClick={() => setSaveOpen(true)}>
          Save layout
        </Button>
        <SaveLayoutDialog
          dashboardId={dashboardId}
          open={saveOpen}
          onOpenChange={setSaveOpen}
        />
      </>
    )
  } else if (canEdit && pinned) {
    controls = (
      <Tooltip>
        <TooltipTrigger render={<span className="inline-flex" />}>
          <Button
            id={editLayoutButtonId}
            variant="outline"
            disabled
            focusableWhenDisabled
          >
            <LayoutGridIcon data-icon="inline-start" />
            Edit layout
          </Button>
        </TooltipTrigger>
        <TooltipContent side="bottom" align="end">
          Open the latest version to edit the layout
        </TooltipContent>
      </Tooltip>
    )
  } else if (canEdit) {
    controls = (
      <Button
        id={editLayoutButtonId}
        variant="outline"
        onClick={() => {
          start(dashboardId, version, spec)
          focusFirstMoveHandle()
        }}
      >
        <LayoutGridIcon data-icon="inline-start" />
        Edit layout
      </Button>
    )
  }

  return (
    <>
      {controls}
      <DiscardLayoutDialog
        open={discardOpen}
        finalFocus={() => document.getElementById(editLayoutButtonId)}
        onKeep={() => setDiscardOpen(false)}
        onDiscard={() => {
          setDiscardOpen(false)
          end()
        }}
      />
    </>
  )
}

export function NewerVersionNotice({
  dashboardId,
  baseVersion,
  latestVersion,
}: {
  dashboardId: string
  baseVersion: number
  latestVersion: number
}) {
  const saving = useIsMutating({
    mutationKey: dashboardKeys.saves(dashboardId),
  })
  if (latestVersion <= baseVersion || saving > 0) return null
  return (
    <div className="px-4 pt-3">
      <Alert>
        <AlertDescription className="text-foreground">
          Version {latestVersion} was saved while you were editing. Saving will
          ask you to reapply your layout to it.
        </AlertDescription>
      </Alert>
    </div>
  )
}
