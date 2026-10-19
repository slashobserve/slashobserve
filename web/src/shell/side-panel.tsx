import {
  createContext,
  useContext,
  useEffect,
  useId,
  useRef,
  type KeyboardEvent,
  type ReactNode,
  type RefObject,
} from "react"
import { createPortal } from "react-dom"
import { XIcon } from "lucide-react"

import { Button } from "@/components/ui/button"

export const PanelHostContext = createContext<HTMLElement | null>(null)

type Props = {
  title: string
  description?: string
  onClose: () => void
  closeDisabled?: boolean
  footer?: ReactNode
  initialFocus?: RefObject<HTMLElement | null>
  children: ReactNode
}

export function SidePanel(props: Props) {
  const host = useContext(PanelHostContext)
  if (!host) return null
  return createPortal(<PanelCard {...props} />, host)
}

function PanelCard({
  title,
  description,
  onClose,
  closeDisabled = false,
  footer,
  initialFocus,
  children,
}: Props) {
  const titleId = useId()
  const asideRef = useRef<HTMLElement>(null)
  const titleRef = useRef<HTMLHeadingElement>(null)

  useEffect(() => {
    const previous = document.activeElement
    ;(initialFocus?.current ?? titleRef.current)?.focus()
    return () => {
      if (previous instanceof HTMLElement && previous.isConnected) {
        previous.focus()
      }
    }
  }, [initialFocus])

  function onKeyDown(event: KeyboardEvent<HTMLElement>) {
    if (event.key !== "Escape" || closeDisabled) return
    if (!asideRef.current?.contains(event.target as Node)) return
    onClose()
  }

  return (
    <aside
      ref={asideRef}
      aria-labelledby={titleId}
      onKeyDown={onKeyDown}
      className="flex min-h-0 max-w-full min-w-0 flex-[0_1_440px] flex-col border-l bg-background @max-[1000px]:grow @max-[1000px]:border-t @max-[1000px]:border-l-0"
    >
      <div className="flex items-start justify-between gap-2 border-b pt-4 pr-3 pb-3.5 pl-[18px]">
        <div className="flex min-w-0 flex-col gap-[3px]">
          <h2
            id={titleId}
            ref={titleRef}
            tabIndex={-1}
            className="text-[15px] font-semibold outline-none"
          >
            {title}
          </h2>
          {description ? (
            <p className="text-[13px] text-muted-foreground">{description}</p>
          ) : null}
        </div>
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label="Close"
          disabled={closeDisabled}
          onClick={onClose}
        >
          <XIcon aria-hidden="true" className="size-[15px]" strokeWidth={1.8} />
        </Button>
      </div>
      <div className="relative flex min-h-0 flex-1 flex-col gap-4 overflow-auto px-[18px] py-4">
        {children}
      </div>
      {footer ? (
        <div className="flex flex-wrap items-center justify-end gap-2 border-t px-[18px] pt-3 pb-3.5">
          {footer}
        </div>
      ) : null}
    </aside>
  )
}
