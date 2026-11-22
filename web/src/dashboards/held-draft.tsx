import { toast } from "sonner"

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

import { useCanvasTool, type DetachedDraft } from "./canvas-tools"

const detachedText = "The spot this draft was on is no longer on the page."

function DetachedBox({ detached }: { detached: DetachedDraft }) {
  const drop = useCanvasTool((state) => state.dropDetached)
  async function copy() {
    try {
      await navigator.clipboard.writeText(detached.text)
      drop()
      toast.success("Your draft is copied.")
    } catch {
      toast.error("The draft could not be copied. Select it and copy it.")
    }
  }
  return (
    <section
      aria-label={
        detached.tool === "ask" ? "Unsent question" : "Unsent comment"
      }
      className="fixed right-4 bottom-4 z-40 flex w-80 flex-col gap-2 rounded-card border border-border bg-popover p-3 text-popover-foreground"
    >
      <p className="text-[13px] text-muted-foreground">{detachedText}</p>
      <p className="max-h-32 overflow-y-auto text-[13px] wrap-anywhere whitespace-pre-wrap select-text">
        {detached.text}
      </p>
      <div className="flex justify-end gap-2">
        <Button variant="ghost" size="sm" onClick={drop}>
          Discard
        </Button>
        <Button variant="outline" size="sm" onClick={() => void copy()}>
          Copy text
        </Button>
      </div>
    </section>
  )
}

export function HeldDraft() {
  const leaving = useCanvasTool((state) => state.leaving)
  const detached = useCanvasTool((state) => state.detached)
  const keepTyping = useCanvasTool((state) => state.keepTyping)
  const discard = useCanvasTool((state) => state.discard)
  return (
    <>
      {detached === null ? null : <DetachedBox detached={detached} />}
      <AlertDialog
        open={leaving !== null}
        onOpenChange={(open) => {
          if (!open) keepTyping()
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Discard your draft?</AlertDialogTitle>
            <AlertDialogDescription>
              The text you typed has not been sent.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Keep editing</AlertDialogCancel>
            <AlertDialogAction onClick={discard}>Discard</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
