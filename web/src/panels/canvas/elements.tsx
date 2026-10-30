import {
  useId,
  type CSSProperties,
  type FocusEvent,
  type PointerEvent,
  type ReactNode,
} from "react"
import { Button as ButtonPrimitive } from "@base-ui/react/button"
import { CircleIcon } from "lucide-react"

import { Button, buttonVariants as buttonClasses } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { anchorProps } from "@/dashboards/dashboard-links"
import { assetImageUrl } from "@/lib/remote-assets"
import { cn } from "@/lib/utils"

import type { ResolvedLink } from "../field-config"
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
import { noActionTitle, type ElementAction } from "./actions"
import type { Interaction, Target } from "./element-links"
import { hasIcon, iconsByName, unknownIcon } from "./icons"
import { ElementShape, shapeKinds } from "./shapes"

export type Hover = { details: Detail[]; x: number; y: number }

type ShowHover = (details: Detail[], clientX: number, clientY: number) => void

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

const variantFor = {
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
  tooltipOn: boolean,
  interaction: Interaction
): boolean {
  if (["text", "metric", "icon", "button"].includes(element.kind)) return true
  if (interaction.mode !== "none") return true
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

const overlayClass =
  "absolute inset-0 cursor-pointer aria-disabled:cursor-not-allowed rounded-[inherit] outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring"

function ButtonFace({ element }: { element: CanvasElement }) {
  const OkIcon = thresholdIcons.ok
  return (
    <>
      {element.buttonStyle === "success" ? (
        <OkIcon aria-hidden="true" style={{ color: "var(--threshold-ok)" }} />
      ) : null}
      <span className="min-w-0 truncate">{element.text ?? "Button"}</span>
    </>
  )
}

function DisabledButton({ element }: { element: CanvasElement }) {
  return (
    <span title={noActionTitle} className="flex size-full min-w-0">
      <Button
        variant={variantFor[element.buttonStyle]}
        disabled
        focusableWhenDisabled
        className="size-full min-w-0"
        style={{ fontSize: element.fontSize }}
      >
        <ButtonFace element={element} />
      </Button>
    </span>
  )
}

function Surface({
  element,
  reason,
  ...props
}: ButtonPrimitive.Props & {
  element: CanvasElement
  reason: string | null
}) {
  const reasonId = useId()
  const blocked = reason !== null
  const described = blocked ? reasonId : undefined
  const note = blocked ? (
    <span id={reasonId} className="sr-only">
      {reason}
    </span>
  ) : null
  if (element.kind === "button") {
    return (
      <span title={reason ?? undefined} className="flex size-full min-w-0">
        <Button
          variant={variantFor[element.buttonStyle]}
          disabled={blocked}
          focusableWhenDisabled
          aria-describedby={described}
          className="size-full min-w-0"
          style={{ fontSize: element.fontSize }}
          {...props}
        >
          <ButtonFace element={element} />
        </Button>
        {note}
      </span>
    )
  }
  return (
    <>
      <ButtonPrimitive
        disabled={blocked}
        focusableWhenDisabled
        title={reason ?? undefined}
        aria-describedby={described}
        className={overlayClass}
        {...props}
      />
      {note}
    </>
  )
}

function LinkSurface({
  element,
  link,
}: {
  element: CanvasElement
  link: ResolvedLink
}) {
  const label = link.newTab ? `${link.title} (opens in new tab)` : link.title
  if (element.kind === "button") {
    return (
      <a
        {...anchorProps(link)}
        className={buttonClasses({
          variant: variantFor[element.buttonStyle],
          className: "size-full min-w-0",
        })}
        style={{ fontSize: element.fontSize }}
      >
        <ButtonFace element={element} />
      </a>
    )
  }
  return (
    <a {...anchorProps(link)} aria-label={label} className={overlayClass} />
  )
}

function targetTitle(target: Target): string {
  return target.kind === "link" ? target.link.title : target.action.title
}

function targetBlocked(target: Target, reason: string | null): boolean {
  return target.kind === "action" && reason !== null
}

function MenuItem({
  target,
  reason,
  onRun,
}: {
  target: Target
  reason: string | null
  onRun: (action: ElementAction) => void
}) {
  if (target.kind === "link") {
    return (
      <DropdownMenuItem render={<a {...anchorProps(target.link)} />}>
        {target.link.title}
      </DropdownMenuItem>
    )
  }
  const blocked = targetBlocked(target, reason)
  return (
    <DropdownMenuItem
      disabled={blocked}
      onClick={() => onRun(target.action)}
      className="flex-col items-start gap-0"
    >
      {target.action.title}
      {blocked ? (
        <span className="text-xs text-muted-foreground">{reason}</span>
      ) : null}
    </DropdownMenuItem>
  )
}

function ElementControl({
  element,
  interaction,
  reason,
  onRun,
}: {
  element: CanvasElement
  interaction: Interaction
  reason: string | null
  onRun: (action: ElementAction) => void
}) {
  if (interaction.mode === "none") return null
  if (interaction.mode === "direct") {
    const { target } = interaction
    if (target.kind === "link") {
      return <LinkSurface element={element} link={target.link} />
    }
    return (
      <Surface
        element={element}
        reason={reason}
        aria-label={element.kind === "button" ? undefined : targetTitle(target)}
        onClick={() => onRun(target.action)}
      />
    )
  }
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Surface
            element={element}
            reason={null}
            aria-label={`${element.text ?? element.id} options`}
          />
        }
      />
      <DropdownMenuContent className="min-w-max">
        {interaction.targets.map((target, index) => (
          <MenuItem
            key={`${index}:${targetTitle(target)}`}
            target={target}
            reason={reason}
            onRun={onRun}
          />
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
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

function Body({
  element,
  control,
}: {
  element: CanvasElement
  control: ReactNode
}): ReactNode {
  if (element.kind === "button") {
    return control ?? <DisabledButton element={element} />
  }
  return (
    <>
      {element.kind === "icon" ? (
        <IconBody element={element} />
      ) : (
        <TextBody element={element} />
      )}
      {control}
    </>
  )
}

export function ContentItem({
  element,
  interaction,
  reason,
  tooltipOn,
  onShow,
  onHide,
  onRun,
}: {
  element: CanvasElement
  interaction: Interaction
  reason: string | null
  tooltipOn: boolean
  onShow: ShowHover
  onHide: () => void
  onRun: (action: ElementAction) => void
}) {
  const controlled = interaction.mode !== "none"
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
      tabIndex={interactive && !controlled ? 0 : undefined}
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
      <Body
        element={element}
        control={
          controlled ? (
            <ElementControl
              element={element}
              interaction={interaction}
              reason={reason}
              onRun={onRun}
            />
          ) : null
        }
      />
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
