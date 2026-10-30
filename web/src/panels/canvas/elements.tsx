import type { CSSProperties, FocusEvent, PointerEvent, ReactNode } from "react"
import { CircleIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { assetImageUrl } from "@/lib/remote-assets"
import { cn } from "@/lib/utils"

import { RemoteImage } from "../remote-image"
import { thresholdIcons } from "../thresholds"
import type {
  Background,
  CanvasAlign,
  CanvasElement,
  CanvasValign,
  ButtonStyle,
  Detail,
  ImageSize,
  MetricState,
} from "./canvas-model"
import { hasIcon, iconsByName, unknownIcon } from "./icons"
import { ElementShape, shapeKinds } from "./shapes"

export type Hover = { details: Detail[]; x: number; y: number }

type ShowHover = (details: Detail[], clientX: number, clientY: number) => void

const noActionTitle = "This button has no action yet."
const tooltipOffset = 12
const tooltipHeight = 120

const justify: Record<CanvasAlign, string> = {
  left: "justify-start text-left",
  center: "justify-center text-center",
  right: "justify-end text-right",
}

const alignItems: Record<CanvasAlign, string> = {
  left: "items-start",
  center: "items-center",
  right: "items-end",
}

const verticalItems: Record<CanvasValign, string> = {
  top: "items-start",
  middle: "items-center",
  bottom: "items-end",
}

const objectFit: Record<
  Exclude<ImageSize, "tile">,
  CSSProperties["objectFit"]
> = {
  original: "none",
  contain: "contain",
  cover: "cover",
  fill: "fill",
}

const buttonVariants = {
  primary: "default",
  secondary: "secondary",
  success: "outline",
  destructive: "destructive",
} as const satisfies Record<ButtonStyle, string>

function cssUrl(href: string): string {
  return `url("${href.replace(/["\\\n]/g, "")}")`
}

function placedStyle(element: CanvasElement): CSSProperties {
  const { left, top, width, height } = element.box
  return {
    left,
    top,
    width,
    height,
    transform:
      element.rotation === 0 ? undefined : `rotate(${element.rotation}deg)`,
  }
}

function detailLabel(details: Detail[]): string {
  return details.map((detail) => `${detail.name}: ${detail.text}`).join(", ")
}

export function BackgroundImage({ background }: { background: Background }) {
  if (background.size === "tile") {
    const href = assetImageUrl(background.url)
    return href === null ? null : (
      <div
        aria-hidden="true"
        className="absolute inset-0"
        style={{ backgroundImage: cssUrl(href), backgroundRepeat: "repeat" }}
      />
    )
  }
  return (
    <RemoteImage
      src={background.url}
      alt=""
      className="absolute inset-0 size-full"
      style={{ objectFit: objectFit[background.size] }}
    />
  )
}

export function hasShape(element: CanvasElement): boolean {
  return shapeKinds.includes(element.kind) || element.background !== null
}

export function drawsContent(
  element: CanvasElement,
  tooltipOn: boolean
): boolean {
  if (["text", "metric", "icon", "button"].includes(element.kind)) return true
  if (element.text !== null) return true
  return (
    element.details.length > 0 && (tooltipOn || element.kind === "windTurbine")
  )
}

export function ShapeItem({ element }: { element: CanvasElement }) {
  return (
    <div
      data-canvas-shape={element.kind}
      data-element-id={element.id}
      className="pointer-events-none absolute"
      style={placedStyle(element)}
    >
      {element.background ? (
        <BackgroundImage background={element.background} />
      ) : null}
      {shapeKinds.includes(element.kind) ? (
        <ElementShape element={element} />
      ) : null}
    </div>
  )
}

function StateMark({ state }: { state: MetricState }) {
  const Icon = state.level === "base" ? CircleIcon : thresholdIcons[state.level]
  return (
    <span
      data-canvas-state={state.level}
      className="flex shrink-0 items-center gap-1 text-[0.8em]"
      style={{ color: state.labelColor ?? undefined }}
    >
      <Icon
        className="size-[1em]"
        strokeWidth={2.4}
        fill={state.level === "base" ? "currentColor" : "none"}
        aria-hidden="true"
        style={{ color: state.color }}
      />
      {state.label}
    </span>
  )
}

function IconBody({ element }: { element: CanvasElement }) {
  if (element.image !== null) {
    return (
      <RemoteImage
        src={element.image}
        alt={element.text ?? ""}
        className="size-full object-contain"
      />
    )
  }
  const Icon =
    element.icon !== null && hasIcon(element.icon)
      ? iconsByName[element.icon]
      : unknownIcon
  return (
    <Icon
      role="img"
      aria-label={element.text ?? element.icon ?? "Icon"}
      className="size-full"
      style={{ color: element.iconColor ?? element.textColor ?? undefined }}
    />
  )
}

function ButtonBody({ element }: { element: CanvasElement }) {
  const OkIcon = thresholdIcons.ok
  return (
    <span title={noActionTitle} className="flex size-full min-w-0">
      <Button
        variant={buttonVariants[element.buttonStyle]}
        disabled
        focusableWhenDisabled
        className="size-full min-w-0"
        style={{ fontSize: element.fontSize }}
      >
        {element.buttonStyle === "success" ? (
          <OkIcon aria-hidden="true" style={{ color: "var(--threshold-ok)" }} />
        ) : null}
        <span className="min-w-0 truncate">{element.text ?? "Button"}</span>
      </Button>
    </span>
  )
}

function TextBody({ element }: { element: CanvasElement }) {
  if (element.kind !== "metric") {
    return element.text === null ? null : (
      <span title={element.text} className="min-w-0 truncate">
        {element.text}
      </span>
    )
  }
  return (
    <div className={cn("flex min-w-0 flex-col", alignItems[element.align])}>
      {element.text === null ? null : (
        <span
          title={element.text}
          className="max-w-full truncate text-[0.8em] opacity-70"
        >
          {element.text}
        </span>
      )}
      <div className="flex max-w-full min-w-0 items-baseline gap-1.5">
        <span
          title={element.valueText ?? undefined}
          data-canvas-value=""
          className="min-w-0 truncate font-semibold tabular-nums"
        >
          {element.valueText}
        </span>
        {element.state ? <StateMark state={element.state} /> : null}
      </div>
    </div>
  )
}

function Body({ element }: { element: CanvasElement }): ReactNode {
  if (element.kind === "icon") return <IconBody element={element} />
  if (element.kind === "button") return <ButtonBody element={element} />
  return <TextBody element={element} />
}

export function ContentItem({
  element,
  tooltipOn,
  onShow,
  onHide,
}: {
  element: CanvasElement
  tooltipOn: boolean
  onShow: ShowHover
  onHide: () => void
}) {
  const boxed = element.kind === "text" || element.kind === "metric"
  const graphic = element.kind === "icon" || element.kind === "button"
  const interactive = tooltipOn && element.details.length > 0
  const show = (event: PointerEvent<HTMLDivElement>) =>
    onShow(element.details, event.clientX, event.clientY)
  const focus = (event: FocusEvent<HTMLDivElement>) => {
    const rect = event.currentTarget.getBoundingClientRect()
    onShow(element.details, rect.left + rect.width / 2, rect.bottom)
  }
  return (
    <div
      data-canvas-element={element.kind}
      data-element-id={element.id}
      role={interactive ? "group" : undefined}
      aria-label={interactive ? detailLabel(element.details) : undefined}
      tabIndex={interactive ? 0 : undefined}
      title={
        !tooltipOn && element.kind === "windTurbine"
          ? detailLabel(element.details)
          : undefined
      }
      className={cn(
        "absolute flex min-w-0 overflow-hidden leading-tight outline-none focus-visible:ring-2 focus-visible:ring-ring",
        !graphic && "px-1",
        justify[element.align],
        verticalItems[element.valign],
        boxed && element.fill && element.ownRadius === null && "rounded-md"
      )}
      style={{
        ...placedStyle(element),
        fontSize: element.fontSize,
        color: element.textColor ?? undefined,
        backgroundColor: boxed ? (element.fill ?? undefined) : undefined,
        borderRadius: element.ownRadius ?? undefined,
      }}
      onPointerEnter={interactive ? show : undefined}
      onPointerMove={interactive ? show : undefined}
      onPointerLeave={interactive ? onHide : undefined}
      onFocus={interactive ? focus : undefined}
      onBlur={interactive ? onHide : undefined}
    >
      <Body element={element} />
    </div>
  )
}

export function Tooltip({
  hover,
  width,
  height,
}: {
  hover: Hover
  width: number
  height: number
}) {
  const onRight = hover.x > width / 2
  return (
    <div
      role="tooltip"
      className="pointer-events-none absolute z-10 max-w-64 min-w-36 rounded-control border border-border bg-popover px-2 py-1.5 text-xs text-popover-foreground"
      style={{
        top: Math.min(
          hover.y + tooltipOffset,
          Math.max(0, height - tooltipHeight)
        ),
        ...(onRight
          ? { right: width - hover.x + tooltipOffset }
          : { left: hover.x + tooltipOffset }),
      }}
    >
      {hover.details.map((detail) => (
        <div key={detail.name} className="flex justify-between gap-3">
          <span className="min-w-0 truncate text-muted-foreground">
            {detail.name}
          </span>
          <span className="min-w-0 truncate font-medium tabular-nums">
            {detail.text}
          </span>
        </div>
      ))}
    </div>
  )
}
