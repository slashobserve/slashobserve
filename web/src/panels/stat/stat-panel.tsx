import { useLayoutEffect, useMemo, useRef, type CSSProperties } from "react"
import { ArrowDownIcon, ArrowUpIcon, CircleIcon, MinusIcon } from "lucide-react"

import { anchorProps, NewTabMark } from "@/dashboards/dashboard-links"
import { useTheme } from "@/lib/theme"
import { cn } from "@/lib/utils"

import { rectWithin, surfaceOf, useAnchorPublisher } from "../anchor-layout"
import { useStableActionRunner } from "../actions/action-runner"
import { itemInteraction } from "../actions/item-interaction"
import { LinksMenu } from "../actions/links-menu"
import { resolveColor } from "../colors"
import { thresholdIcons } from "../thresholds"
import type { PanelRenderer } from "../types"
import type { ValueAnchorLayout } from "../value-anchors"
import { cssColorToRgb, readableText } from "../contrast"
import { Sparkline } from "./sparkline"
import { sparklineHasArea, sparklinePoints } from "./sparkline-points"
import {
  fittedValueSize,
  isWideTile,
  rowMinHeight,
  tileGap,
  tilesAcross,
  tileSize,
} from "./stat-grid"
import {
  changeTone,
  statScope,
  statTiles,
  type ChangeDirection,
  type ChangeTone,
  type StatDisplay,
  type StatTile,
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

function gridFor(
  orientation: string,
  across: number,
  rowMin: number
): CSSProperties {
  const rows = `minmax(${rowMin}px, 1fr)`
  switch (orientation) {
    case "horizontal":
      return {
        gridAutoFlow: "column",
        gridAutoColumns: "minmax(120px, 1fr)",
        gridTemplateRows: rows,
      }
    case "vertical":
      return {
        gridTemplateColumns: "minmax(0, 1fr)",
        gridAutoRows: rows,
      }
    default:
      return {
        gridTemplateColumns: `repeat(${across}, minmax(0, 1fr))`,
        gridAutoRows: rows,
      }
  }
}

const StatPanel: PanelRenderer = ({
  panel,
  frames,
  fields,
  display: rawDisplay,
  zone,
  meta,
  width,
  height,
  links,
}) => {
  const { resolved: theme } = useTheme()
  const { runner, dialog } = useStableActionRunner(panel.id, meta)
  const display = rawDisplay as StatDisplay
  const tiles = useMemo(
    () => statTiles(frames, fields, display, panel.unit, zone),
    [frames, fields, display, panel.unit, zone]
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
  const size = tileSize(width, height, across, tiles.length)
  const wide = isWideTile(display.wideLayout, size)
  const rowCount =
    display.orientation === "horizontal"
      ? 1
      : display.orientation === "vertical"
        ? tiles.length
        : Math.ceil(tiles.length / across)
  const rowMin = rowMinHeight(height, rowCount)
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
  const gridRef = useRef<HTMLDivElement>(null)
  const publish = useAnchorPublisher("stat")
  useLayoutEffect(() => {
    const grid = gridRef.current
    if (grid === null) return
    const measure = () =>
      publish(statAnchorLayout(grid, tiles, { width, height }))
    measure()
    grid.addEventListener("scroll", measure, { passive: true })
    return () => grid.removeEventListener("scroll", measure)
  }, [publish, tiles, width, height])
  const textMode = display.textMode
  const showText = textMode !== "none"
  const nameAsValue = textMode === "name"
  const showName =
    textMode === "valueAndName" || (textMode === undefined && tiles.length > 1)
  const sideBySide = wide && showName && showText
  const pad = sideBySide ? "" : "px-3"
  const valueStyleFor = (text: string, unit: string) => {
    if (display.valueSize) return { fontSize: display.valueSize }
    if (sideBySide) return undefined
    const room = size.width - 24 - (unit ? unit.length * 8 + 6 : 0)
    const fitted = fittedValueSize(text, room)
    return fitted < 28 ? { fontSize: fitted } : undefined
  }

  return (
    <>
      <div
        ref={gridRef}
        className={cn("grid gap-2 overflow-auto", anyFilled && "p-2 pt-0")}
        style={{
          width,
          height,
          ...gridFor(display.orientation, across, rowMin),
        }}
      >
        {tiles.map((tile, index) => {
          const filled = background && tile.color !== null
          const aria = tiles.length > 1 ? tile.name : panel.title
          const line = lines[index]
          const drawn = line.length > 0
          const primary = nameAsValue ? tile.name : tile.amount
          const unit = nameAsValue ? "" : tile.unit
          const valueStyle = valueStyleFor(primary, unit)
          const {
            links: dataLinks,
            actions,
            oneClick,
            run,
          } = itemInteraction(tile, statScope(tile, links), runner, showText)
          const linkedPrimary = showText ? dataLinks[0] : undefined
          const menuLinks = showText ? dataLinks.slice(1) : dataLinks
          const thresholdColor =
            display.colorMode === "value" ? tile.color : null
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
              {...oneClick}
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
                        style={
                          display.percentSize
                            ? { fontSize: display.percentSize }
                            : undefined
                        }
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
                    area={sparklineHasArea(filled, display.graphStyle)}
                  />
                </div>
              ) : null}
              {menuLinks.length > 0 || actions.length > 0 ? (
                <LinksMenu
                  links={menuLinks}
                  actions={actions}
                  reason={runner.reason}
                  onAction={run}
                  subject={tile.name}
                  className="absolute top-1 right-1 text-current"
                  linkIcon
                />
              ) : null}
            </div>
          )
        })}
      </div>
      {dialog}
    </>
  )
}

function statAnchorLayout(
  grid: HTMLElement,
  tiles: readonly StatTile[],
  body: { width: number; height: number }
): ValueAnchorLayout {
  const root = surfaceOf(grid)
  const drawn = grid.querySelectorAll(":scope > [data-stat-tile]")
  return {
    body,
    targets: tiles.flatMap((tile, index) => {
      const element = drawn.item(index)
      if (element === null) return []
      return [
        {
          rect: rectWithin(element, root),
          name: tile.name,
          labels: tile.labels,
          text: tile.text,
        },
      ]
    }),
  }
}

export default StatPanel
