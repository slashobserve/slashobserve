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

import { useActivityStore, usePanelLayout } from "@/activity/activity-store"
import { DockHeader } from "@/components/dock/dock-header"
import { DockPanel } from "@/components/dock/dock-panel"

export const PanelHostContext = createContext<HTMLElement | null>(null)

type Props = {
  title: string
  description?: string
  onClose: () => void
  closeDisabled?: boolean
  hidden?: boolean
  footer?: ReactNode
  initialFocus?: RefObject<HTMLElement | null>
  trigger?: RefObject<HTMLElement | null>
  titleRef?: RefObject<HTMLHeadingElement | null>
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
  hidden,
  footer,
  initialFocus,
  trigger,
  titleRef: titleRefProp,
  children,
}: Props) {
  const titleId = useId()
  const asideRef = useRef<HTMLElement>(null)
  const ownTitleRef = useRef<HTMLHeadingElement>(null)
  const titleRef = titleRefProp ?? ownTitleRef
  const layout = usePanelLayout()
  const setPanelWidth = useActivityStore((state) => state.setPanelWidth)

  useEffect(() => {
    const previous = document.activeElement
    const triggerElement = trigger?.current
    ;(initialFocus?.current ?? titleRef.current)?.focus()
    return () => {
      const target =
        previous instanceof HTMLElement &&
        previous !== document.body &&
        previous.isConnected
          ? previous
          : triggerElement
      target?.focus()
    }
  }, [initialFocus, trigger, titleRef])

  function onKeyDown(event: KeyboardEvent<HTMLElement>) {
    if (event.key !== "Escape" || closeDisabled) return
    if (!asideRef.current?.contains(event.target as Node)) return
    onClose()
  }

  return (
    <DockPanel
      ref={asideRef}
      hidden={hidden}
      labelledBy={titleId}
      onKeyDown={onKeyDown}
      width={layout.width}
      bounds={layout.bounds}
      onWidthChange={setPanelWidth}
      stacked={layout.stacked}
    >
      <DockHeader
        headingId={titleId}
        headingRef={titleRef}
        title={title}
        description={description}
        onClose={onClose}
        closeLabel="Close"
        closeDisabled={closeDisabled}
      />
      <div className="relative flex min-h-0 flex-1 flex-col gap-4 overflow-auto p-4">
        {children}
      </div>
      {footer ? (
        <div className="flex flex-none flex-wrap items-center justify-end gap-2 border-t px-4 pt-3 pb-3.5">
          {footer}
        </div>
      ) : null}
    </DockPanel>
  )
}
