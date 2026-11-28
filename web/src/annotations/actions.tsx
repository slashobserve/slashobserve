import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useState,
  type ReactNode,
} from "react"
import { LoaderCircleIcon } from "lucide-react"
import { toast } from "sonner"

import { loadProblemText } from "@/api/errors"
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
import { roleAtLeast, type Role } from "@/lib/roles"

import { AnnotationDialog, type Draft } from "./annotation-dialog"
import { useDeleteAnnotation } from "./mutations"
import type { Annotation } from "./queries"

export type NewAnnotation = {
  fromMs: number
  toMs?: number
  panelId?: string
}

export type AnnotationActions = {
  canAdd: boolean
  add: (where: NewAnnotation) => void
  canChange: (annotation: Annotation) => boolean
  edit: (annotation: Annotation) => void
  remove: (annotation: Annotation) => void
}

const ActionsContext = createContext<AnnotationActions | null>(null)

export function useAnnotationActions(): AnnotationActions | null {
  return useContext(ActionsContext)
}

type ProviderProps = {
  dashboardId: string
  zone: string | undefined
  panelTitle: (id: string) => string
  canEdit: boolean
  principal: { id: string; role: Role }
  children: ReactNode
}

export function AnnotationActionsProvider({
  dashboardId,
  zone,
  panelTitle,
  canEdit,
  principal,
  children,
}: ProviderProps) {
  const [draft, setDraft] = useState<Draft | null>(null)
  const [deleting, setDeleting] = useState<Annotation | null>(null)
  const { id: principalId, role } = principal
  const admin = roleAtLeast(role, "admin")

  const add = useCallback(
    ({ fromMs, toMs, panelId }: NewAnnotation) =>
      setDraft({ kind: "new", fromMs, toMs: toMs ?? null, panelId }),
    []
  )
  const canChange = useCallback(
    (annotation: Annotation) =>
      admin ||
      (annotation.created_by !== null && annotation.created_by === principalId),
    [admin, principalId]
  )
  const edit = useCallback(
    (annotation: Annotation) => setDraft({ kind: "edit", annotation }),
    []
  )

  const value = useMemo<AnnotationActions>(
    () => ({
      canAdd: canEdit,
      add,
      canChange,
      edit,
      remove: setDeleting,
    }),
    [canEdit, add, canChange, edit]
  )

  return (
    <ActionsContext.Provider value={value}>
      {children}
      {draft ? (
        <AnnotationDialog
          draft={draft}
          dashboardId={dashboardId}
          zone={zone}
          panelTitle={panelTitle}
          canGlobal={roleAtLeast(role, "editor")}
          onClose={() => setDraft(null)}
        />
      ) : null}
      {deleting ? (
        <DeleteAnnotationDialog
          annotation={deleting}
          onClose={() => setDeleting(null)}
        />
      ) : null}
    </ActionsContext.Provider>
  )
}

function DeleteAnnotationDialog({
  annotation,
  onClose,
}: {
  annotation: Annotation
  onClose: () => void
}) {
  const remove = useDeleteAnnotation(annotation.id)
  const pending = remove.isPending

  function confirm() {
    remove.mutate(undefined, {
      onSuccess: () => {
        toast.success("Annotation deleted")
        onClose()
      },
    })
  }

  return (
    <AlertDialog
      open
      onOpenChange={(open) => (open || pending ? null : onClose())}
    >
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Delete this annotation?</AlertDialogTitle>
          <AlertDialogDescription className="wrap-anywhere">
            {annotation.dashboard_id === null
              ? "It disappears from every dashboard. "
              : "It disappears from this dashboard. "}
            This cannot be undone.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <p className="text-sm break-words whitespace-pre-wrap">
          {annotation.text}
        </p>
        {remove.isError ? (
          <Alert variant="destructive">
            <AlertDescription>{loadProblemText(remove.error)}</AlertDescription>
          </Alert>
        ) : null}
        <AlertDialogFooter>
          <AlertDialogCancel aria-disabled={pending}>Cancel</AlertDialogCancel>
          <AlertDialogAction
            variant="destructive"
            disabled={pending}
            focusableWhenDisabled
            aria-busy={pending}
            onClick={confirm}
          >
            {pending ? (
              <LoaderCircleIcon
                aria-hidden="true"
                className="motion-safe:animate-spin"
              />
            ) : null}
            {remove.isError ? "Try again" : "Delete"}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
