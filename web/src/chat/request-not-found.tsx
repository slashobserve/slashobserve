import { Link } from "@tanstack/react-router"

import { buttonVariants } from "@/components/ui/button"
import { cn } from "@/lib/utils"
import { cannotOpenChatText, cannotOpenChatTitle } from "@/observe/not-openable"

export function RequestNotFound({
  removed,
  pane,
}: {
  removed: boolean
  pane: boolean
}) {
  const text = removed ? "This chat no longer exists." : cannotOpenChatText
  if (pane) {
    return (
      <p role="alert" className="text-[13px] text-muted-foreground">
        {cannotOpenChatTitle}. {text}
      </p>
    )
  }
  return (
    <div className="flex min-h-full flex-col items-center justify-center gap-4 text-center">
      <h1 className="text-2xl font-semibold">{cannotOpenChatTitle}</h1>
      <p className="max-w-[44ch] text-sm text-muted-foreground">{text}</p>
      <Link to="/ask" className={cn(buttonVariants({ variant: "outline" }))}>
        Back to Observe
      </Link>
    </div>
  )
}
