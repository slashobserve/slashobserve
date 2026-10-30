import { useMemo, type CSSProperties } from "react"
import {
  ArrowDownIcon,
  ArrowUpIcon,
  CircleIcon,
  LinkIcon,
  MinusIcon,
} from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  anchorProps,
  NewTabMark,
  oneClickProps,
} from "@/dashboards/dashboard-links"
import { useTheme } from "@/lib/theme"
import { cn } from "@/lib/utils"

import { resolveColor } from "../colors"
import { isDataLink, resolveLink } from "../field-config"
import { thresholdIcons } from "../thresholds"
import type { PanelRenderer } from "../types"
import { cssColorToRgb, readableText } from "../contrast"
import { Sparkline } from "./sparkline"
import { sparklinePoints } from "./sparkline-points"
import { isWideTile, tileGap, tilesAcross, tileSize } from "./stat-grid"
import {
  changeTone,
  statTiles,
  type ChangeDirection,
  type ChangeTone,
  type StatDisplay,
} from "./stat-tiles"

const valueClass =
  "min-w-0 truncate text-[28px] font-semibold tracking-[-0.02em] tabular-nums"

const changeIcons: Record<ChangeDirection, typeof ArrowUpIcon> = {
  up: ArrowUpIcon,
  down: ArrowDownIcon,
  flat: MinusIcon,
}

const changeColors: Record<Exclude<ChangeTone, "value" | "none">, string> = {
  ok: "var(--threshold-ok)",
  critical: "var(--threshold-critical)",
}

function gridFor(orientation: string, across: number): CSSProperties {
  switch (orientation) {
    case "horizontal":
      return {
        gridAutoFlow: "column",
        gridAutoColumns: "minmax(120px, 1fr)",
        gridTemplateRows: "minmax(72px, 1fr)",
      }
    case "vertical":
      return {
        gridTemplateColumns: "minmax(0, 1fr)",
        gridAutoRows: "minmax(72px, 1fr)",
      }
    default:
      return {
        gridTemplateColumns: `repeat(${across}, minmax(0, 1fr))`,
        gridAutoRows: "minmax(72px, 1fr)",
      }
  }
}

const StatPanel: PanelRenderer = ({
  panel,
  frames,
  fields,
  display: rawDisplay,
  width,
  height,
  links,
}) => {
  const { resolved: theme } = useTheme()
  const display = rawDisplay as StatDisplay
  const tiles = useMemo(
    () => statTiles(frames, fields, display, panel.unit),
    [frames, fields, display, panel.unit]
  )
  const background = display.colorMode === "background"
  const anyFilled = background && tiles.some((tile) => tile.color !== null)
  const across = tilesAcross(
    display.orientation,
    width,
    anyFilled ? height - tileGap : height,
    tiles.length
  )
  const columns = Math.ceil(width / across)
  const wide = isWideTile(
    display.wideLayout,
    tileSize(width, height, across, tiles.length)
  )
  const centered = display.justify === "center"
  const lines = useMemo(
    () =>
      tiles.map((tile) =>
        display.graph && tile.points.length >= 2
          ? sparklinePoints(tile.points, columns)
          : []
      ),
    [tiles, display.graph, columns]
  )
  const filledText = useMemo(
    () =>
      tiles.map((tile) => {
        if (display.colorMode !== "background" || !tile.color) return undefined
        const rgb = cssColorToRgb(resolveColor(tile.color))
        return rgb ? readableText(rgb) : undefined
      }),
    // eslint-disable-next-line react-hooks/exhaustive-deps -- resolveColor reads the live theme tokens; panel-stat.spec "recomputes the text color when the theme flips" fails without theme here
    [tiles, display.colorMode, theme]
  )
  const textMode = display.textMode
  const showText = textMode !== "none"
  const nameAsValue = textMode === "name"
  const showName =
    textMode === "valueAndName" || (textMode === undefined && tiles.length > 1)
  const sideBySide = wide && showName && showText
  const pad = sideBySide ? "" : "px-3"
  const valueStyle = display.valueSize
    ? { fontSize: display.valueSize }
    : undefined

  return (
    <div
      className={cn("grid gap-2 overflow-auto", anyFilled && "p-2 pt-0")}
      style={{ width, height, ...gridFor(display.orientation, across) }}
    >
      {tiles.map((tile, index) => {
        const filled = background && tile.color !== null
        const aria = tiles.length > 1 ? tile.name : panel.title
        const line = lines[index]
        const drawn = line.length > 0
        const primary = nameAsValue ? tile.name : tile.amount
        const unit = nameAsValue ? "" : tile.unit
        const dataLinks = tile.links.flatMap(
          (link) =>
            (isDataLink(link)
              ? resolveLink(link, {
                  ...links,
                  labels: tile.labels,
                  seriesName: tile.seriesName,
                  fieldName: tile.fieldName,
                  value: tile.value,
                  valueText: tile.text,
                })
              : null) ?? []
        )
        const linkedPrimary = showText ? dataLinks[0] : undefined
        const menuLinks = showText ? dataLinks.slice(1) : dataLinks
        const thresholdColor = display.colorMode === "value" ? tile.color : null
        const Icon =
          tile.threshold.level !== "base"
            ? thresholdIcons[tile.threshold.level]
            : thresholdColor
              ? CircleIcon
              : null
        const change = display.percentChange && showText ? tile.change : null
        const ChangeIcon = changeIcons[change?.direction ?? "flat"]
        const tone = change
          ? changeTone(
              change.direction,
              display.percentChangeColor ?? "standard"
            )
          : "none"
        const changeColor = filled
          ? undefined
          : tone === "value"
            ? (tile.color ?? undefined)
            : tone === "none"
              ? undefined
              : changeColors[tone]
        const sparkStroke = filled
          ? "currentColor"
          : (thresholdColor ?? "var(--series-1)")
        return (
          <div
            key={tile.key}
            role="group"
            aria-label={`${aria}: ${tile.text}${tile.threshold.level !== "base" ? `, ${tile.threshold.label}` : ""}`}
            data-stat-tile=""
            {...oneClickProps(showText ? dataLinks : [])}
            data-color-mode={display.colorMode}
            data-graph={String(drawn)}
            data-threshold={tile.threshold.level}
            className={cn(
              "relative flex min-h-0 flex-col overflow-hidden rounded-md text-foreground data-one-click:cursor-pointer",
              filled && "pt-2"
            )}
            style={
              filled
                ? {
                    backgroundColor: tile.color ?? undefined,
                    color: filledText[index],
                  }
                : undefined
            }
          >
            <div
              className={cn(
                "flex shrink-0 flex-col",
                sideBySide &&
                  "flex-row items-baseline justify-between gap-3 px-3",
                sideBySide && centered && "justify-center"
              )}
            >
              {showName ? (
                <span
                  title={tile.name}
                  style={
                    display.titleSize
                      ? { fontSize: display.titleSize }
                      : undefined
                  }
                  className={cn(
                    "shrink-0 truncate text-[13px] font-medium",
                    sideBySide ? "min-w-0 shrink" : "px-3",
                    centered && !sideBySide && "text-center",
                    !filled && "text-muted-foreground"
                  )}
                >
                  {tile.name}
                </span>
              ) : null}
              {showText ? (
                <div
                  className={cn(
                    "flex shrink-0 items-baseline gap-1.5 leading-none",
                    pad,
                    centered && !sideBySide && "justify-center"
                  )}
                >
                  {linkedPrimary ? (
                    <a
                      {...anchorProps(linkedPrimary)}
                      data-stat-value=""
                      title={linkedPrimary.title}
                      aria-label={`${primary}${unit ? ` ${unit}` : ""}: ${linkedPrimary.title}${linkedPrimary.newTab ? " (opens in new tab)" : ""}`}
                      style={valueStyle}
                      className={cn(
                        valueClass,
                        "-mx-1 rounded-sm px-1 underline-offset-4 outline-none hover:underline focus-visible:ring-2 focus-visible:ring-inset"
                      )}
                    >
                      {primary}
                      {linkedPrimary.newTab ? <NewTabMark /> : null}
                    </a>
                  ) : (
                    <span
                      data-stat-value=""
                      style={valueStyle}
                      className={valueClass}
                    >
                      {primary}
                    </span>
                  )}
                  {unit ? (
                    <span
                      data-stat-unit=""
                      className={cn(
                        "shrink-0 text-[13px]",
                        !filled && "text-muted-foreground"
                      )}
                    >
                      {unit}
                    </span>
                  ) : null}
                  {change ? (
                    <span
                      data-stat-change=""
                      data-direction={change.direction}
                      className={cn(
                        "flex shrink-0 items-center gap-0.5 text-[13px]",
                        !filled && "text-muted-foreground"
                      )}
                    >
                      <ChangeIcon
                        className="size-[13px]"
                        strokeWidth={2.4}
                        aria-hidden="true"
                        style={{ color: changeColor }}
                      />
                      <span className="sr-only">{change.word} </span>
                      {change.text}
                    </span>
                  ) : null}
                </div>
              ) : null}
            </div>
            {Icon && showText ? (
              <span
                className={cn(
                  "flex shrink-0 items-center gap-1.5 px-3 text-[12px]",
                  centered && "justify-center",
                  !filled && "text-muted-foreground"
                )}
              >
                <Icon
                  className="size-[13px]"
                  strokeWidth={2.4}
                  fill={
                    tile.threshold.level === "base" ? "currentColor" : "none"
                  }
                  aria-hidden="true"
                  data-stat-icon=""
                  style={{ color: thresholdColor ?? undefined }}
                />
                {tile.threshold.label}
              </span>
            ) : null}
            {drawn ? (
              <div className="mt-auto h-8 min-h-0 shrink">
                <Sparkline
                  line={line}
                  stroke={sparkStroke}
                  strokeOpacity={filled ? 0.5 : 1}
                  area={!filled}
                />
              </div>
            ) : null}
            {menuLinks.length > 0 ? (
              <DropdownMenu>
                <DropdownMenuTrigger
                  render={
                    <Button
                      variant="ghost"
                      size="icon-xs"
                      className="absolute top-1 right-1 text-current"
                      aria-label={`More links for ${tile.name}`}
                    />
                  }
                >
                  <LinkIcon aria-hidden="true" />
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end" className="min-w-max">
                  {menuLinks.map((link, linkIndex) => (
                    <DropdownMenuItem
                      key={`${linkIndex}:${link.href}`}
                      render={<a {...anchorProps(link)} />}
                      className="justify-between gap-3"
                    >
                      {link.title}
                      {link.newTab ? <NewTabMark /> : null}
                    </DropdownMenuItem>
                  ))}
                </DropdownMenuContent>
              </DropdownMenu>
            ) : null}
          </div>
        )
      })}
    </div>
  )
}

export default StatPanel
