import { useState } from "react"
import { useIsMutating } from "@tanstack/react-query"
import { XIcon } from "lucide-react"

import { dashboardKeys } from "@/api/keys"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"

import {
  editLayoutButtonId,
  focusEditLayoutButton,
  useIsDirty,
  useLayoutStore,
} from "./layout-store"
import { SaveLayoutDialog } from "./save-layout-dialog"
import { DiscardLayoutDialog } from "./unsaved-changes-guard"

type Props = {
  dashboardId: string
  pinned: boolean
}

export function findFirstMoveHandle(): HTMLElement | null {
  return document.querySelector<HTMLElement>("button.panel-drag-handle")
}

export function EditControls({ dashboardId, pinned }: Props) {
  const sessionOpen = useLayoutStore(
    (state) => state.session?.dashboardId === dashboardId
  )
  const editing = sessionOpen && !pinned
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

  return (
    <>
      {editing ? (
        <>
          {dirty ? (
            <Badge variant="secondary" className="max-sm:hidden">
              Unsaved changes
            </Badge>
          ) : null}
          <Button
            variant="ghost"
            className="h-[30px] max-sm:w-8 max-sm:px-0"
            onClick={cancel}
          >
            <XIcon className="sm:hidden" />
            <span className="max-sm:sr-only">Cancel</span>
          </Button>
          <Button
            className="h-[30px]"
            disabled={!dirty}
            onClick={() => setSaveOpen(true)}
          >
            Save layout
          </Button>
          <SaveLayoutDialog
            dashboardId={dashboardId}
            open={saveOpen}
            onOpenChange={setSaveOpen}
          />
        </>
      ) : null}
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
