import { sparklineHeight, sparklineWidth } from "./sparkline-points"

export function Sparkline({
  line,
  stroke,
  strokeOpacity = 1,
  area,
}: {
  line: readonly string[]
  stroke: string
  strokeOpacity?: number
  area: boolean
}) {
  return (
    <svg
      aria-hidden="true"
      viewBox={`0 0 ${sparklineWidth} ${sparklineHeight}`}
      preserveAspectRatio="none"
      className="block size-full"
    >
      {area ? (
        <polygon
          points={`0,${sparklineHeight} ${line.join(" ")} ${sparklineWidth},${sparklineHeight}`}
          style={{ fill: stroke, opacity: 0.12 }}
        />
      ) : null}
      <polyline
        points={line.join(" ")}
        fill="none"
        strokeWidth={1.5}
        strokeLinejoin="round"
        vectorEffect="non-scaling-stroke"
        style={{ stroke, strokeOpacity }}
      />
    </svg>
  )
}
