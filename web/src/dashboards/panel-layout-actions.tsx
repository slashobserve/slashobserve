import { useCallback, useMemo, useState } from "react"
import { LoaderCircleIcon } from "lucide-react"
import { toast } from "sonner"

import { Alert, AlertDescription } from "@/components/ui/alert"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { panelTitle, type Dashboard, type LayoutPanel } from "@/spec"

import { saveProblemFrom } from "./edit/save-problem"
import { duplicatePanelPatch, removePanelPatch } from "./panel-layout-patch"
import { usePatchPanels } from "./queries"
import { saveToast } from "./save-outcome"

export type PanelLayoutActions = {
  duplicate: (panel: LayoutPanel) => void
  remove: (panel: LayoutPanel) => Promise<string | null>
  pending: boolean
}

const conflictText =
  "The dashboard changed since you opened it; reload and try again"

function problemText(failure: unknown, heading: string): string {
  const problem = saveProblemFrom(failure, heading)
  if (problem.kind === "conflict") return conflictText
  if (problem.kind === "invalid") {
    return [problem.heading, ...problem.lines].join(" ")
  }
  return problem.message
}

export function usePanelLayoutActions(
  dashboardId: string,
  spec: Dashboard,
  version: number
): PanelLayoutActions {
  const { mutate, mutateAsync, isPending } = usePatchPanels(dashboardId)
  const duplicate = useCallback(
    (panel: LayoutPanel) => {
      mutate(
        {
          patch: duplicatePanelPatch(spec, panel),
          summary: `Duplicate "${panelTitle(panel)}"`,
          parent_version: version,
          acceptEmpty: true,
        },
        {
          onSuccess: (outcome) => toast.success(saveToast(outcome)),
          onError: (failure) =>
            toast.error(problemText(failure, "Could not duplicate the panel.")),
        }
      )
    },
    [mutate, spec, version]
  )
  const remove = useCallback(
    async (panel: LayoutPanel) => {
      try {
        const outcome = await mutateAsync({
          patch: removePanelPatch(panel),
          summary: `Remove "${panelTitle(panel)}"`,
          parent_version: version,
        })
        toast.success(saveToast(outcome))
        return null
      } catch (failure) {
        return problemText(failure, "Could not remove the panel.")
      }
    },
    [mutateAsync, version]
  )
  return useMemo(
    () => ({ duplicate, remove, pending: isPending }),
    [duplicate, remove, isPending]
  )
}

export function RemovePanelDialog({
  panel,
  actions,
  onClose,
}: {
  panel: LayoutPanel
  actions: PanelLayoutActions
  onClose: () => void
}) {
  const [problem, setProblem] = useState<string | null>(null)
  const title = panelTitle(panel)
  async function confirm() {
    const failure = await actions.remove(panel)
    if (failure === null) onClose()
    else setProblem(failure)
  }
  return (
    <AlertDialog
      open
      onOpenChange={(open) => {
        if (!open && !actions.pending) onClose()
      }}
    >
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Remove "{title}"?</AlertDialogTitle>
          <AlertDialogDescription>
            This saves a new version without the panel. Restore the version
            before it from History to bring it back.
          </AlertDialogDescription>
        </AlertDialogHeader>
        {problem !== null ? (
          <Alert variant="destructive">
            <AlertDescription>{problem}</AlertDescription>
          </Alert>
        ) : null}
        <AlertDialogFooter>
          <AlertDialogCancel aria-disabled={actions.pending}>
            Cancel
          </AlertDialogCancel>
          <AlertDialogAction
            variant="destructive"
            disabled={actions.pending}
            focusableWhenDisabled
            aria-busy={actions.pending}
            onClick={() => void confirm()}
          >
            {actions.pending ? (
              <LoaderCircleIcon
                aria-hidden="true"
                className="motion-safe:animate-spin"
              />
            ) : null}
            Remove panel
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
