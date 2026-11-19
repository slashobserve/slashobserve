import { useCoarsePointer } from "@/lib/coarse-pointer"

function sendHintText(coarse: boolean, action: "send" | "save"): string {
  return coarse
    ? `Tap the arrow to ${action}.`
    : `Enter to ${action}, Shift+Enter for a new line.`
}

export function SendHint({ action = "send" }: { action?: "send" | "save" }) {
  const coarse = useCoarsePointer()
  return (
    <p className="text-xs text-muted-foreground">
      {sendHintText(coarse, action)}
    </p>
  )
}
