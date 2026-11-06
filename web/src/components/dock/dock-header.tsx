import type { ReactNode, Ref } from "react"
import { ArrowLeftIcon, XIcon } from "lucide-react"

import { Button } from "@/components/ui/button"

type Props = {
  headingId: string
  headingRef: Ref<HTMLHeadingElement>
  title: string
  description?: string
  back?: () => void
  controls?: ReactNode
  onClose?: () => void
  closeLabel: string
  closeDisabled?: boolean
}

export function DockHeader({
  headingId,
  headingRef,
  title,
  description,
  back,
  controls,
  onClose,
  closeLabel,
  closeDisabled = false,
}: Props) {
  return (
    <div
      data-slot="dock-header"
      className="visible flex flex-none items-start justify-between gap-2 border-b pt-3.5 pr-3 pb-3 pl-4 group-data-[peek]/dock:items-center group-data-[peek]/dock:border-b-0 group-data-[peek]/dock:pb-2 group-data-[sheet]/dock:pt-0"
    >
      <div className="flex min-w-0 flex-col gap-0.5">
        <h2
          id={headingId}
          ref={headingRef}
          tabIndex={-1}
          className="text-[15px] font-semibold wrap-anywhere outline-none group-data-[peek]/dock:truncate"
        >
          {title}
        </h2>
        {description ? (
          <p
            title={description}
            className="truncate text-[13px] text-muted-foreground group-data-[peek]/dock:hidden"
          >
            {description}
          </p>
        ) : null}
      </div>
      <div className="flex flex-none items-center gap-1.5">
        {back ? (
          <Button
            variant="ghost"
            size="sm"
            onClick={back}
            className="group-data-[peek]/dock:hidden"
          >
            <ArrowLeftIcon aria-hidden="true" />
            Back to activity
          </Button>
        ) : null}
        {controls ? (
          <div className="contents group-data-[peek]/dock:hidden">
            {controls}
          </div>
        ) : null}
        {onClose ? (
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={closeLabel}
            disabled={closeDisabled}
            onClick={onClose}
          >
            <XIcon aria-hidden="true" className="size-[15px]" />
          </Button>
        ) : null}
      </div>
    </div>
  )
}
