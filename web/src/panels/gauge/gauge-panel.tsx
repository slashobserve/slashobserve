import { useMemo } from "react"
import { EllipsisIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { anchorProps, NewTabMark } from "@/dashboards/dashboard-links"

import type { FieldResolver, LinkScope, ResolvedLink } from "../field-config"
import { thresholdIcons } from "../thresholds"
import type { PanelRenderer } from "../types"
import {
  gaugeLinks,
  gaugeModel,
  valueFontSize,
  type Gauge,
} from "./gauge-model"
import { gaugeGrid, gridGap } from "./gauge-grid"
import { GaugeSvg } from "./gauge-svg"

const bodyInsetX = 16
const bodyInsetY = 8

function MoreLinks({ links }: { links: ResolvedLink[] }) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            variant="ghost"
            size="icon-xs"
            className="text-muted-foreground"
            aria-label="More links"
          />
        }
      >
        <EllipsisIcon aria-hidden="true" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="min-w-max">
        {links.map((link, index) => (
          <DropdownMenuItem
            key={`${index}:${link.href}`}
            render={<a {...anchorProps(link)} />}
            className="justify-between gap-3"
          >
            {link.title}
            {link.newTab ? <NewTabMark /> : null}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

type CellProps = {
  gauge: Gauge
  named: boolean
  fields: FieldResolver
  scope: Omit<
    LinkScope,
    "row" | "seriesName" | "fieldName" | "value" | "valueText"
  >
}

function GaugeCell({ gauge, named, fields, scope }: CellProps) {
  const [first, ...more] = gaugeLinks(gauge, fields, scope)
  const { level, label: word } = gauge.threshold
  const Icon = level === "base" ? null : thresholdIcons[level]
  return (
    <div
      role="meter"
      aria-label={gauge.name}
      aria-valuemin={gauge.min}
      aria-valuemax={gauge.max}
      aria-valuenow={gauge.value ?? undefined}
      aria-valuetext={word ? `${gauge.text}, ${word}` : gauge.text}
      data-gauge={gauge.name}
      data-markers={gauge.markers.length > 0}
      data-threshold={level}
      className="[container-type:size] relative min-h-0 min-w-0"
    >
      <GaugeSvg gauge={gauge} />
      <div
        className="absolute inset-0 flex flex-col items-center justify-center gap-[1.5cqmin]"
        style={{ paddingTop: "16cqmin" }}
      >
        <span className="inline-flex items-center gap-1">
          <span
            className="leading-none font-semibold text-foreground tabular-nums"
            style={{ fontSize: `${valueFontSize(gauge.text).toFixed(2)}cqmin` }}
          >
            {first ? (
              <a
                {...anchorProps(first)}
                title={first.title || undefined}
                className="underline decoration-border underline-offset-4 hover:decoration-current"
              >
                {gauge.text}
              </a>
            ) : (
              gauge.text
            )}
          </span>
          {first?.newTab ? <NewTabMark /> : null}
          {more.length > 0 ? <MoreLinks links={more} /> : null}
        </span>
        {named ? (
          <span
            title={gauge.name}
            className="max-w-[70cqmin] truncate text-muted-foreground"
            style={{ fontSize: "clamp(11px, 7cqmin, 16px)" }}
          >
            {gauge.name}
          </span>
        ) : null}
        {Icon ? (
          <span
            className="flex items-center gap-1 font-medium text-foreground"
            style={{ fontSize: "clamp(11px, 6.5cqmin, 14px)" }}
          >
            <Icon
              aria-hidden="true"
              style={{
                color: gauge.threshold.color ?? undefined,
                width: "1.2em",
                height: "1.2em",
              }}
            />
            {word}
          </span>
        ) : null}
      </div>
    </div>
  )
}

export const GaugePanel: PanelRenderer = ({
  frames,
  fields,
  display,
  links,
  width,
  height,
}) => {
  const gauges = useMemo(
    () => gaugeModel(frames, fields, display),
    [frames, fields, display]
  )
  const { columns, rows } = gaugeGrid(
    gauges.length,
    width - bodyInsetX,
    height - bodyInsetY
  )
  return (
    <div
      className="grid overflow-hidden p-2 pt-0"
      style={{
        width,
        height,
        gap: gridGap,
        gridTemplateColumns: `repeat(${columns}, minmax(0, 1fr))`,
        gridTemplateRows: `repeat(${rows}, minmax(0, 1fr))`,
      }}
    >
      {gauges.map((gauge) => (
        <GaugeCell
          key={gauge.key}
          gauge={gauge}
          named={gauges.length > 1}
          fields={fields}
          scope={links}
        />
      ))}
    </div>
  )
}

export default GaugePanel
