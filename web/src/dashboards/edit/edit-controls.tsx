import { useState } from "react"
import { LayoutGridIcon, XIcon } from "lucide-react"

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
  const editing = useLayoutStore(
    (state) => state.session?.dashboardId === dashboardId
  )
  const start = useLayoutStore((state) => state.start)
  const end = useLayoutStore((state) => state.end)
  const dirty = useIsDirty()
  const [saveOpen, setSaveOpen] = useState(false)
  const [discardOpen, setDiscardOpen] = useState(false)

  if (!editing) {
    if (!canEdit) return null
    if (pinned) {
      return (
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
    }
    return (
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

  function cancel() {
    if (dirty) {
      setDiscardOpen(true)
      return
    }
    end()
    focusEditLayoutButton()
  }

  return (
    <>
      {dirty ? (
        <Badge variant="secondary" className="max-sm:hidden">
          Unsaved changes
        </Badge>
      ) : null}
      <Button variant="ghost" onClick={cancel}>
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
      <DiscardLayoutDialog
        open={discardOpen}
        onKeep={() => setDiscardOpen(false)}
        onDiscard={() => {
          setDiscardOpen(false)
          end()
          focusEditLayoutButton()
        }}
      />
    </>
  )
}

export function NewerVersionNotice({
  baseVersion,
  latestVersion,
}: {
  baseVersion: number
  latestVersion: number
}) {
  if (latestVersion <= baseVersion) return null
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
