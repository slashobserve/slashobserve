import { useMemo, type CSSProperties } from "react"
import { LinkIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { anchorProps, NewTabMark } from "@/dashboards/dashboard-links"
import { useTheme } from "@/lib/theme"

import { resolveColor } from "../colors"
import { isDataLink, resolveLink } from "../field-config"
import { thresholdIcons } from "../thresholds"
import type { PanelRenderer } from "../types"
import { cssColorToRgb, readableText } from "./contrast"
import { Sparkline } from "./sparkline"
import { statDisplay, statTiles } from "./stat-tiles"

const charWidth = 0.62
const sparklineHeight = "min(40cqh, max(0px, calc(100cqh - 60px)))"

function fontSizeFor(text: string): string {
  const columns = Math.max(text.length, 3) * charWidth
  return `clamp(1.25rem, min(${(94 / columns).toFixed(1)}cqw, 45cqh), 3.5rem)`
}

function useRerenderOnThemeChange() {
  useTheme()
}

function gridFor(orientation: string): CSSProperties {
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
        gridTemplateColumns: "repeat(auto-fit, minmax(120px, 1fr))",
        gridAutoRows: "minmax(72px, 1fr)",
      }
  }
}

export const StatPanel: PanelRenderer = ({
  panel,
  frames,
  fields,
  display: rawDisplay,
  width,
  height,
  links,
}) => {
  useRerenderOnThemeChange()
  const display = useMemo(() => statDisplay(rawDisplay), [rawDisplay])
  const tiles = useMemo(
    () => statTiles(frames, fields, display, panel.unit),
    [frames, fields, display, panel.unit]
  )
  const backgroundText = tiles.map((tile) => {
    if (display.colorMode !== "background" || !tile.color) return null
    const rgb = cssColorToRgb(resolveColor(tile.color))
    return rgb ? readableText(rgb) : null
  })
  const textMode = display.textMode
  const showText = textMode !== "none"
  const nameAsValue = textMode === "name"
  const showName =
    textMode === "valueAndName" || (textMode === undefined && tiles.length > 1)

  return (
    <div
      className="grid gap-2 overflow-auto p-2 pt-0"
      style={{ width, height, ...gridFor(display.orientation) }}
    >
      {tiles.map((tile, index) => {
        const filledText = backgroundText[index]
        const filled = filledText !== null
        const Icon =
          tile.threshold.level === "base"
            ? null
            : thresholdIcons[tile.threshold.level]
        const aria = tiles.length > 1 ? tile.name : panel.title
        const drawn = display.graph && tile.points.length >= 2
        const primary = nameAsValue ? tile.name : tile.text
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
        const sparkStroke = filled
          ? filledText
          : (tile.color ?? "var(--series-1)")
        return (
          <div
            key={tile.key}
            role="group"
            aria-label={`${aria}: ${tile.text}${tile.threshold.level !== "base" ? `, ${tile.threshold.label}` : ""}`}
            data-stat-tile=""
            data-color-mode={display.colorMode}
            data-graph={String(drawn)}
            data-threshold={tile.threshold.level}
            className="[container-type:size] relative flex min-h-0 flex-col overflow-hidden rounded-md px-2 text-foreground"
            style={
              filled
                ? {
                    backgroundColor: tile.color ?? undefined,
                    color: filledText,
                  }
                : undefined
            }
          >
            <div className="[container-type:size] flex min-h-0 flex-1 flex-col items-center justify-center gap-1">
              {showName ? (
                <span
                  title={tile.name}
                  className={`max-w-full truncate text-xs ${filled ? "opacity-80" : "text-muted-foreground"}`}
                >
                  {tile.name}
                </span>
              ) : null}
              {showText ? (
                <span
                  className="max-w-full truncate leading-none font-semibold tabular-nums"
                  style={{
                    fontSize: fontSizeFor(primary),
                    color:
                      display.colorMode === "value" && tile.color
                        ? tile.color
                        : undefined,
                  }}
                >
                  {linkedPrimary ? (
                    <a
                      {...anchorProps(linkedPrimary)}
                      className="underline-offset-4 hover:underline"
                    >
                      {primary}
                      {linkedPrimary.newTab ? <NewTabMark /> : null}
                    </a>
                  ) : (
                    primary
                  )}
                </span>
              ) : null}
              {Icon && showText ? (
                <span className="flex items-center gap-1 text-xs font-medium">
                  <Icon
                    className="size-4"
                    aria-hidden="true"
                    style={{
                      color: filled ? undefined : (tile.color ?? undefined),
                    }}
                  />
                  {tile.threshold.label}
                </span>
              ) : null}
            </div>
            {drawn ? (
              <div className="shrink-0" style={{ height: sparklineHeight }}>
                <Sparkline
                  points={tile.points}
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
