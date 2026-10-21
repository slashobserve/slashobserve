import type { ReactNode, Ref } from "react"
import { ArrowLeftIcon, XIcon } from "lucide-react"

import { Button } from "@/components/ui/button"

type HeaderProps = {
  headingId: string
  headingRef: Ref<HTMLHeadingElement>
  title: string
  description: string
  back?: () => void
  controls?: ReactNode
  close: () => void
}

export function CardHeader({
  headingId,
  headingRef,
  title,
  description,
  back,
  controls,
  close,
}: HeaderProps) {
  return (
    <div className="flex items-start justify-between gap-2 border-b pt-3.5 pr-3 pb-3 pl-4">
      <div className="flex min-w-0 flex-col gap-0.5">
        <h2
          id={headingId}
          ref={headingRef}
          tabIndex={-1}
          className="text-[15px] font-semibold wrap-anywhere outline-none"
        >
          {title}
        </h2>
        <p className="text-[13px] text-muted-foreground">{description}</p>
      </div>
      <div className="flex flex-none items-center gap-1.5">
        {back ? (
          <Button variant="ghost" size="sm" onClick={back}>
            <ArrowLeftIcon aria-hidden="true" />
            Back to activity
          </Button>
        ) : null}
        {controls}
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label="Close panel"
          onClick={close}
        >
          <XIcon aria-hidden="true" className="size-[15px]" />
        </Button>
      </div>
    </div>
  )
}
