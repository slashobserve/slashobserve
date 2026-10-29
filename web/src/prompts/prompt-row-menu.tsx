import { useEffect, useRef, useState, type RefObject } from "react"
import { EllipsisIcon, LoaderCircleIcon } from "lucide-react"
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
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"

import { usePromptPanel } from "./prompt-panel-store"
import {
  promptProblem,
  useDeletePrompt,
  useUpdatePrompt,
  type SavedPrompt,
} from "./queries"

type Props = {
  prompt: SavedPrompt
  headingRef: RefObject<HTMLHeadingElement | null>
}

export function PromptRowMenu({ prompt, headingRef }: Props) {
  const triggerId = `prompt-menu-${prompt.id}`
  const openPanel = usePromptPanel((state) => state.openPanel)
  const update = useUpdatePrompt()
  const remove = useDeletePrompt()
  const [confirming, setConfirming] = useState(false)
  const [problem, setProblem] = useState<string | null>(null)
  const deleted = useRef(false)
  const leaveFocusAlone = useRef(false)

  useEffect(
    () => () => {
      const active = document.activeElement
      if (deleted.current && (active === null || active === document.body)) {
        headingRef.current?.focus()
      }
    },
    [headingRef]
  )

  function toggleShared() {
    const shared = !prompt.shared
    update.mutate(
      { id: prompt.id, patch: { shared } },
      {
        onSuccess: () =>
          toast.success(
            shared
              ? `"${prompt.name}" is now shared with everyone.`
              : `"${prompt.name}" is now private.`
          ),
        onError: (error) =>
          toast.error(promptProblem(error, { name: prompt.name, shared }).text),
      }
    )
  }

  function changeConfirming(next: boolean) {
    if (!next && remove.isPending) return
    setConfirming(next)
    if (!next) {
      setProblem(null)
      remove.reset()
    }
  }

  function confirmDelete() {
    setProblem(null)
    remove.mutate(prompt.id, {
      onSuccess: () => {
        deleted.current = true
        toast.success(`Deleted "${prompt.name}"`)
        setConfirming(false)
      },
      onError: (error) =>
        setProblem(
          promptProblem(error, { name: prompt.name, shared: false }).text
        ),
    })
  }

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            <Button
              id={triggerId}
              variant="ghost"
              size="icon-sm"
              aria-label={`More for ${prompt.name}`}
            />
          }
        >
          <EllipsisIcon aria-hidden="true" />
        </DropdownMenuTrigger>
        <DropdownMenuContent
          align="end"
          className="min-w-max"
          finalFocus={() => {
            const leave = leaveFocusAlone.current
            leaveFocusAlone.current = false
            return !leave
          }}
        >
          <DropdownMenuItem
            onClick={() => {
              leaveFocusAlone.current = true
              openPanel({
                kind: "edit",
                promptId: prompt.id,
                returnFocusId: triggerId,
              })
            }}
          >
            Edit
          </DropdownMenuItem>
          <DropdownMenuItem disabled={update.isPending} onClick={toggleShared}>
            {prompt.shared ? "Make private" : "Share with everyone"}
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem
            variant="destructive"
            onClick={() => {
              leaveFocusAlone.current = true
              deleted.current = false
              setConfirming(true)
            }}
          >
            Delete...
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <AlertDialog open={confirming} onOpenChange={changeConfirming}>
        <AlertDialogContent
          finalFocus={() =>
            deleted.current
              ? headingRef.current
              : document.getElementById(triggerId)
          }
        >
          <AlertDialogHeader>
            <AlertDialogTitle className="wrap-anywhere">
              Delete &quot;{prompt.name}&quot;?
            </AlertDialogTitle>
            <AlertDialogDescription>
              {prompt.shared
                ? "Everyone on this server loses it, and observe run stops finding it. This cannot be undone."
                : "This cannot be undone."}
            </AlertDialogDescription>
          </AlertDialogHeader>
          {problem !== null ? (
            <Alert variant="destructive">
              <AlertDescription>{problem}</AlertDescription>
            </Alert>
          ) : null}
          <AlertDialogFooter>
            <AlertDialogCancel aria-disabled={remove.isPending}>
              Cancel
            </AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              disabled={remove.isPending}
              focusableWhenDisabled
              aria-busy={remove.isPending}
              onClick={confirmDelete}
            >
              {remove.isPending ? (
                <LoaderCircleIcon
                  aria-hidden="true"
                  className="motion-safe:animate-spin"
                />
              ) : null}
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
